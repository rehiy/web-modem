package services

import (
	"errors"
	"fmt"
	"log"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/tarm/serial"

	"modem-manager/models"
	"modem-manager/utils"
)

// SerialService 封装单个串口的读写与监听。
type SerialService struct {
	name       string
	port       *serial.Port
	lock       *sync.Mutex
}

func newSerialService(name string, port *serial.Port) *SerialService {
	return &SerialService{
		name:       name,
		port:       port,
		lock:       &sync.Mutex{},
	}
}

// readLoop 持续读取串口输出并广播。
func (s *SerialService) readLoop() {
	buf := make([]byte, 128)
	for {
		s.lock.Lock()
		n, err := s.port.Read(buf)
		s.lock.Unlock()
		if err != nil {
			time.Sleep(150 * time.Millisecond)
			continue
		}
		if n > 0 {
			GetGlobalListener().broadcast("[" + s.name + "] " + string(buf[:n]))
		}
	}
}

// SendATCommand 发送 AT 命令并读取响应。
func (s *SerialService) SendATCommand(command string) (string, error) {
	return s.sendRawCommand(command, "\r\n")
}

// sendRawCommand 发送原始命令并读取响应。
func (s *SerialService) sendRawCommand(command, append string) (string, error) {
	s.lock.Lock()
	defer s.lock.Unlock()

	// 清理历史未读数据，避免旧数据干扰本次响应
	_ = s.port.Flush()

	_, err := s.port.Write([]byte(command + append))
	if err != nil {
		return "", err
	}

	response := ""
	buf := make([]byte, 128)
	// 采用滚动超时：只要持续有数据到达，就延长等待窗口
	deadline := time.Now().Add(5 * time.Second)
	lastData := time.Now()

	for {
		n, err := s.port.Read(buf)
		if err != nil && err.Error() != "EOF" {
			// 若长时间没有新数据则超时退出
			if time.Since(lastData) > 5*time.Second || time.Now().After(deadline) {
				if response == "" {
					return response, errors.New("timeout")
				}
				return response, nil
			}
			time.Sleep(50 * time.Millisecond)
			continue
		}

		if n > 0 {
			response += string(buf[:n])
			lastData = time.Now()
			deadline = lastData.Add(5 * time.Second)

			if strings.Contains(response, "OK") || strings.Contains(response, "ERROR") {
				return response, nil
			}
			if strings.Contains(response, ">") {
				return response, nil
			}
			continue
		}

		// 无数据时检查是否超时
		if time.Since(lastData) > 5*time.Second || time.Now().After(deadline) {
			if response == "" {
				return response, errors.New("timeout")
			}
			return response, nil
		}
		time.Sleep(50 * time.Millisecond)
	}
}

// GetModemInfo 获取当前端口的基本信息。
func (s *SerialService) GetModemInfo() (*models.ModemInfo, error) {
	info := &models.ModemInfo{
		Port:      s.name,
		Connected: true,
	}

	if resp, err := s.SendATCommand("AT+CGMI"); err == nil {
		info.Manufacturer = extractValue(resp)
	}
	if resp, err := s.SendATCommand("AT+CGMM"); err == nil {
		info.Model = extractValue(resp)
	}
	if resp, err := s.SendATCommand("AT+CGSN"); err == nil {
		info.IMEI = extractValue(resp)
	}
	if resp, err := s.SendATCommand("AT+CIMI"); err == nil {
		info.IMSI = extractValue(resp)
	}
	if resp, err := s.SendATCommand("AT+CNUM"); err == nil {
		info.PhoneNumber = extractPhoneNumber(resp)
	}
	if resp, err := s.SendATCommand("AT+COPS?"); err == nil {
		info.Operator = extractOperator(resp)
	}

	return info, nil
}

// GetSignalStrength 查询信号强度。
func (s *SerialService) GetSignalStrength() (*models.SignalStrength, error) {
	resp, err := s.SendATCommand("AT+CSQ")
	if err != nil {
		return nil, err
	}

	re := regexp.MustCompile(`\+CSQ:\s*(\d+),(\d+)`)
	matches := re.FindStringSubmatch(resp)
	if len(matches) < 3 {
		return nil, errors.New("invalid response")
	}

	rssi := 0
	fmt.Sscanf(matches[1], "%d", &rssi)

	quality := 0
	fmt.Sscanf(matches[2], "%d", &quality)

	dbm := fmt.Sprintf("%d dBm", -113+rssi*2)

	return &models.SignalStrength{RSSI: rssi, Quality: quality, DBM: dbm}, nil
}

// ListSMS 列出当前端口的短信（PDU 模式）。
func (s *SerialService) ListSMS() ([]models.SMS, error) {
	resp, err := s.SendATCommand("AT+CMGL=4")
	if err != nil {
		return nil, err
	}
	return s.parsePDUSMSList(resp), nil
}

// parsePDUSMSList 解析 PDU 模式下的短信列表。
func (s *SerialService) parsePDUSMSList(response string) []models.SMS {
	smsList := []models.SMS{}
	lines := strings.Split(response, "\n")

	for i := 0; i < len(lines); i++ {
		line := strings.TrimSpace(lines[i])
		if strings.HasPrefix(line, "+CMGL:  ") {
			re := regexp.MustCompile(`\+CMGL:\s*(\d+),(\d+),.*,(\d+)`)
			matches := re.FindStringSubmatch(line)

			if len(matches) >= 3 && i+1 < len(lines) {
				index := 0
				fmt.Sscanf(matches[1], "%d", &index)

				pduData := strings.TrimSpace(lines[i+1])
				phone, message, timestamp, err := utils.ParsePDUMessage(pduData)
				if err != nil {
					log.Println("PDU parse error:", err)
					continue
				}

				sms := models.SMS{Index: index, Status: "READ", Number: phone, Time: timestamp, Message: message}
				smsList = append(smsList, sms)
				i++
			}
		}
	}

	return smsList
}

// SendSMS 发送短信（支持长短信、中文）。
func (s *SerialService) SendSMS(number, message string) error {
	pdus := utils.CreatePDUMessage(number, message)
	log.Printf("Sending SMS in %d part(s)", len(pdus))

	for i, pdu := range pdus {
		log.Printf("Sending part %d/%d", i+1, len(pdus))

		pduLen := (len(pdu) - 2) / 2
		cmd := fmt.Sprintf("AT+CMGS=%d", pduLen)
		resp, err := s.SendATCommand(cmd)
		if err != nil {
			return fmt.Errorf("failed to initiate SMS: %v", err)
		}

		if !strings.Contains(resp, ">") {
			return errors.New("modem did not respond with prompt")
		}

		time.Sleep(200 * time.Millisecond)

		_, err = s.sendRawCommand(pdu, "\x1A")
		if err != nil {
			return fmt.Errorf("failed to send PDU: %v", err)
		}

		time.Sleep(2 * time.Second)
	}

	log.Println("SMS sent successfully")
	return nil
}

func extractValue(response string) string {
	lines := strings.Split(response, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line != "" && line != "OK" && !strings.HasPrefix(line, "AT") {
			return line
		}
	}
	return ""
}

func extractOperator(response string) string {
	re := regexp.MustCompile(`"([^"]+)"`)
	matches := re.FindStringSubmatch(response)
	if len(matches) > 1 {
		return matches[1]
	}
	return ""
}

func extractPhoneNumber(response string) string {
	re := regexp.MustCompile(`\+CNUM:\s*"[^"]*",\s*"([^"]+)"`)
	matches := re.FindStringSubmatch(response)
	if len(matches) > 1 {
		return matches[1]
	}

	for _, line := range strings.Split(response, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "AT") || line == "OK" {
			continue
		}
		if strings.ContainsAny(line, "0123456789") {
			return line
		}
	}

	return ""
}
