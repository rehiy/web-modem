package services

import (
	"encoding/hex"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode/utf16"

	"github.com/tarm/serial"

	"modem-manager/models"
)

// SerialService 封装单个串口的读写与监听。
type SerialService struct {
	name string
	port *serial.Port
	mu   *sync.Mutex
}

// NewSerialService 尝试连接并初始化串口服务
func NewSerialService(name string, baudRate int) (*SerialService, error) {
	cfg := &serial.Config{
		Name:        name,
		Baud:        baudRate,
		ReadTimeout: 2 * time.Second,
		Size:        8,
		Parity:      serial.ParityNone,
		StopBits:    serial.Stop1,
	}

	port, err := serial.OpenPort(cfg)
	if err != nil {
		return nil, err
	}

	// 初始化 Modem
	s := &SerialService{name: name, port: port, mu: &sync.Mutex{}}
	if err := s.check(); err != nil {
		port.Close()
		return nil, err
	}

	return s, nil
}

// check 发送基本 AT 命令以验证连接。
func (s *SerialService) check() error {
	resp, err := s.SendATCommand("AT")
	if err != nil {
		return err
	}
	if !strings.Contains(resp, "OK") {
		return fmt.Errorf("command AT failed: %s", resp)
	}
	return nil
}

// Start 启动串口服务的读取循环。
func (s *SerialService) Start() {
	// 关闭回显
	s.SendATCommand("ATE0")
	// 设置文本模式
	s.SendATCommand("AT+CMGF=1")
	// 启动读取循环
	go s.readLoop()
}

// readLoop 持续读取串口输出并广播。
func (s *SerialService) readLoop() {
	buf := make([]byte, 128)
	for {
		s.mu.Lock()
		n, err := s.port.Read(buf)
		s.mu.Unlock()
		if err != nil {
			time.Sleep(150 * time.Millisecond)
			continue
		}
		if n > 0 {
			GetEventListener().Broadcast("[" + s.name + "] " + string(buf[:n]))
		}
	}
}

// SendATCommand 发送 AT 命令并读取响应。
func (s *SerialService) SendATCommand(command string) (string, error) {
	return s.sendRawCommand(command, "\r\n")
}

// sendRawCommand 发送原始命令并读取响应。
func (s *SerialService) sendRawCommand(command, suffix string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// 清理历史未读数据，避免旧数据干扰本次响应
	_ = s.port.Flush()

	_, err := s.port.Write([]byte(command + suffix))
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
	if resp, err := s.SendATCommand("AT+COPS?"); err == nil {
		info.Operator = extractOperator(resp)
	}
	if resp, err := s.GetPhoneNumber(); err == nil {
		info.PhoneNumber = resp
	}

	return info, nil
}

// GetPhoneNumber 查询电话号码。
func (s *SerialService) GetPhoneNumber() (string, error) {
	resp, err := s.SendATCommand("AT+CNUM")
	if err != nil {
		return "", err
	}

	re := regexp.MustCompile(`\+CNUM:\s*"[^"]*",\s*"([^"]+)"`)
	matches := re.FindStringSubmatch(resp)
	if len(matches) > 1 {
		return matches[1], nil
	}

	for _, line := range strings.Split(resp, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "AT") || line == "OK" {
			continue
		}
		if strings.ContainsAny(line, "0123456789") {
			return line, nil
		}
	}

	return "", errors.New("phone number not found")
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

// ListSMS 获取短信列表
func (s *SerialService) ListSMS() ([]models.SMS, error) {
	// 获取所有短信
	resp, err := s.SendATCommand("AT+CMGL=\"ALL\"")
	if err != nil {
		return nil, fmt.Errorf("获取短信失败: %v", err)
	}

	var smsList []models.SMS
	lines := strings.Split(resp, "\n")

	// 正则表达式匹配 +CMGL: index,status,oa,alpha,scts
	// 例如: +CMGL: 1,"REC READ","+8613800138000",,"23/12/18,10:00:00+32"
	re := regexp.MustCompile(`\+CMGL:\s*(\d+),"([^"]*)","([^"]*)",(?:[^,]*,)?"([^"]*)"`)

	// 临时存储所有短信分片
	type smsPart struct {
		models.SMS
		Ref   int
		Total int
		Seq   int
	}
	var allParts []smsPart

	for i := 0; i < len(lines); i++ {
		line := strings.TrimSpace(lines[i])
		if line == "" {
			continue
		}

		matches := re.FindStringSubmatch(line)
		if len(matches) >= 5 {
			index := 0
			fmt.Sscanf(matches[1], "%d", &index)

			// 读取短信内容
			content := ""
			for j := i + 1; j < len(lines); j++ {
				nextLine := strings.TrimSpace(lines[j])
				// 如果遇到下一个 +CMGL 或 OK，则停止
				if strings.HasPrefix(nextLine, "+CMGL:") || nextLine == "OK" {
					i = j - 1
					break
				}
				if content != "" {
					content += "\n"
				}
				content += nextLine
				// 如果是最后一行，更新 i
				if j == len(lines)-1 {
					i = j
				}
			}

			text, ref, total, seq := decodeHexSMS(content)

			allParts = append(allParts, smsPart{
				SMS: models.SMS{
					Index:   index,
					Status:  matches[2],
					Number:  matches[3],
					Time:    matches[4],
					Message: text,
				},
				Ref:   ref,
				Total: total,
				Seq:   seq,
			})
		}
	}

	// 合并长短信
	longSMSMap := make(map[string][]smsPart)
	for _, p := range allParts {
		if p.Total <= 1 {
			smsList = append(smsList, p.SMS)
		} else {
			key := fmt.Sprintf("%s_%d", p.Number, p.Ref)
			longSMSMap[key] = append(longSMSMap[key], p)
		}
	}

	for _, parts := range longSMSMap {
		// 按序号排序
		sort.Slice(parts, func(i, j int) bool {
			return parts[i].Seq < parts[j].Seq
		})

		// 拼接内容
		fullMsg := ""
		for _, part := range parts {
			fullMsg += part.Message
		}

		// 使用第一条分片的信息作为合并后的短信信息
		combined := parts[0].SMS
		combined.Message = fullMsg
		smsList = append(smsList, combined)
	}

	// 按索引排序
	sort.Slice(smsList, func(i, j int) bool {
		return smsList[i].Index < smsList[j].Index
	})

	return smsList, nil
}

// SendSMS 发送短信
func (s *SerialService) SendSMS(number, message string) error {
	// 直接发送短信命令
	cmd := fmt.Sprintf("AT+CMGS=\"%s\"", number)
	resp, err := s.SendATCommand(cmd)
	if err != nil {
		return err
	}

	// 检查是否收到提示符
	if !strings.Contains(resp, ">") {
		return fmt.Errorf("未收到发送提示符")
	}

	// 发送消息内容并以Ctrl+Z结束
	_, err = s.sendRawCommand(message, "\x1A")
	if err != nil {
		return err
	}

	// 简单等待发送完成
	time.Sleep(2 * time.Second)
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

// decodeHexSMS 尝试解码可能是 Hex 格式的短信内容
// 自动处理 UDH 头和 UCS2 编码
// 返回: 解码后的内容, 引用号, 总分片数, 当前分片序号
func decodeHexSMS(content string) (string, int, int, int) {
	// 移除可能的空白字符
	cleanContent := strings.TrimSpace(content)

	// 检查是否全是 Hex 字符且长度为偶数
	if len(cleanContent) == 0 || len(cleanContent)%2 != 0 {
		return content, 0, 1, 1
	}
	for _, c := range cleanContent {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) {
			return content, 0, 1, 1
		}
	}

	// 尝试 Hex Decode
	bytes, err := hex.DecodeString(cleanContent)
	if err != nil {
		return content, 0, 1, 1
	}

	// 检查是否有 UDH (User Data Header)
	// 常见的长短信 UDH:
	// 05 00 03 ref total seq (6 bytes)
	// 06 08 04 ref ref total seq (7 bytes)
	offset := 0
	ref := 0
	total := 1
	seq := 1

	if len(bytes) > 6 && bytes[0] == 0x05 && bytes[1] == 0x00 && bytes[2] == 0x03 {
		offset = 6
		ref = int(bytes[3])
		total = int(bytes[4])
		seq = int(bytes[5])
	} else if len(bytes) > 7 && bytes[0] == 0x06 && bytes[1] == 0x08 && bytes[2] == 0x04 {
		offset = 7
		ref = int(bytes[3])<<8 | int(bytes[4])
		total = int(bytes[5])
		seq = int(bytes[6])
	}

	// 剩下的字节尝试 UCS2 (UTF-16BE) 解码
	if (len(bytes)-offset)%2 != 0 {
		// 长度不对，可能不是 UCS2
		return content, 0, 1, 1
	}

	utf16Codes := make([]uint16, (len(bytes)-offset)/2)
	for i := 0; i < len(utf16Codes); i++ {
		idx := offset + i*2
		utf16Codes[i] = uint16(bytes[idx])<<8 | uint16(bytes[idx+1])
	}

	// 解码 UTF-16
	decoded := string(utf16.Decode(utf16Codes))

	// 如果解码结果为空，返回原始内容
	if decoded == "" {
		return content, 0, 1, 1
	}

	return decoded, ref, total, seq
}
