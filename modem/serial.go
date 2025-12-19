package modem

import (
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/tarm/serial"
)

// SerialService 封装了单个串口的读取、写入和监控。
type SerialService struct {
	name         string
	port         *serial.Port
	broadcast    func(string)
	sync.Mutex
}

// NewSerialService 尝试连接并初始化串口服务。
func NewSerialService(name string, baudRate int, broadcast func(string)) (*SerialService, error) {
	port, err := serial.OpenPort(&serial.Config{
		Name: name, Baud: baudRate, ReadTimeout: readTimeout,
	})
	if err != nil {
		return nil, err
	}

	s := &SerialService{name: name, port: port, broadcast: broadcast}
	if err := s.check(); err != nil {
		port.Close()
		return nil, err
	}
	return s, nil
}

// check 发送基本的 AT 命令以验证连接。
func (s *SerialService) check() error {
	resp, err := s.SendATCommand(cmdCheck)
	if err != nil {
		return err
	}
	if !strings.Contains(resp, respOK) {
		return fmt.Errorf("command AT failed: %s", resp)
	}
	return nil
}

// Start 开始串口服务读取循环。
func (s *SerialService) Start() {
	s.SendATCommand(cmdEchoOff)  // 关闭回显
	s.SendATCommand(cmdTextMode) // 设置文本模式
	go s.readLoop()
}

// readLoop 持续读取串口输出并广播它。
func (s *SerialService) readLoop() {
	buf := make([]byte, bufferSize)
	for {
		s.Lock()
		n, err := s.port.Read(buf)
		s.Unlock()

		if n > 0 && s.broadcast != nil {
			s.broadcast(fmt.Sprintf("[%s] %s", s.name, string(buf[:n])))
		}

		if err != nil {
			time.Sleep(errorSleep)
		}
	}
}

// SendATCommand 发送 AT 命令并读取响应。
func (s *SerialService) SendATCommand(command string) (string, error) {
	return s.sendRawCommand(command, eof, 1*time.Second)
}

// sendRawCommand 发送原始命令并读取响应。
func (s *SerialService) sendRawCommand(command, suffix string, timeout time.Duration) (string, error) {
	s.Lock()
	defer s.Unlock()

	_ = s.port.Flush()
	if _, err := s.port.Write([]byte(command + suffix)); err != nil {
		return "", err
	}

	var resp strings.Builder
	buf := make([]byte, bufferSize)

	start := time.Now()
	for {
		if time.Since(start) > timeout {
			return "", errTimeout
		}

		n, err := s.port.Read(buf)
		if n > 0 {
			resp.Write(buf[:n])
			str := resp.String()
			if strings.Contains(str, respOK) || strings.Contains(str, respError) || strings.Contains(str, ">") {
				return strings.TrimSpace(str), nil
			}
		}
		if err != nil {
			if err == io.EOF {
				time.Sleep(errorSleep)
				continue
			}
			if resp.Len() > 0 {
				return resp.String(), nil
			}
			return "", err
		}
	}
}

// GetModemInfo 获取有关当前端口的基本信息。
func (s *SerialService) GetModemInfo() (*ModemInfo, error) {
	info := &ModemInfo{Port: s.name, Connected: true}
	cmds := map[*string]string{
		&info.Manufacturer: cmdManufacturer,
		&info.Model:        cmdModel,
		&info.IMEI:         cmdIMEI,
		&info.IMSI:         cmdIMSI,
	}

	for ptr, cmd := range cmds {
		if resp, err := s.SendATCommand(cmd); err == nil {
			*ptr = extractValue(resp)
		}
	}

	if resp, err := s.SendATCommand(cmdOperator); err == nil {
		info.Operator = extractOperator(resp)
	}

	info.PhoneNumber, _ = s.GetPhoneNumber()
	return info, nil
}

// GetPhoneNumber 查询电话号码。
func (s *SerialService) GetPhoneNumber() (string, error) {
	resp, err := s.SendATCommand(cmdNumber)
	if err != nil {
		return "", err
	}
	if m := rePhoneNumber.FindStringSubmatch(resp); len(m) > 1 {
		return DecodeUCS2Hex(m[1]), nil
	}
	return "", errNotFound
}

// GetSignalStrength 查询信号强度。
func (s *SerialService) GetSignalStrength() (*SignalStrength, error) {
	resp, err := s.SendATCommand(cmdSignal)
	if err != nil {
		return nil, err
	}

	var rssi, qual int
	if _, err := fmt.Sscanf(extractValue(resp), "+CSQ: %d,%d", &rssi, &qual); err != nil {
		return nil, err
	}

	return &SignalStrength{
		RSSI:    rssi,
		Quality: qual,
		DBM:     fmt.Sprintf("%d dBm", -113+rssi*2),
	}, nil
}

// ListSMS 获取短信列表。
func (s *SerialService) ListSMS() ([]SMS, error) {
	resp, err := s.SendATCommand(cmdListSMS)
	if err != nil {
		return nil, err
	}

	var parts []struct { SMS; ref, total, seq int }

	// 按 +CMGL: 分割以处理多条消息，跳过第一个空部分
	chunks := strings.Split(resp, "+CMGL: ")
	for _, chunk := range chunks[1:] {
		lines := strings.SplitN(chunk, "\n", 2)
		if len(lines) < 2 {
			continue
		}

		// 解析元数据: index,stat,,length
		fields := strings.Split(strings.TrimSpace(lines[0]), ",")
		if len(fields) < 2 {
			continue
		}

		idx, _ := strconv.Atoi(strings.TrimSpace(fields[0]))
		stat, _ := strconv.Atoi(strings.TrimSpace(fields[1]))
		pdu := strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(lines[1]), respOK))

		sender, timestamp, message, ref, total, seq, err := decodePDU(pdu)
		if err != nil {
			// 如果解析失败，保留原始 PDU 以便调试
			message = "PDU Decode Error: " + err.Error() + " Raw: " + pdu
		}

		parts = append(parts, struct{ SMS; ref, total, seq int }{
			SMS: SMS{
				Index:   idx,
				Status:  getPDUStatus(stat),
				Number:  sender,
				Time:    timestamp,
				Message: message,
			},
			ref: ref, total: total, seq: seq,
		})
	}

	// 合并长短信
	var result []SMS
	merged := make(map[string][]struct{ seq int; msg string })

	for _, p := range parts {
		if p.total <= 1 {
			result = append(result, p.SMS)
			continue
		}
		key := fmt.Sprintf("%s_%d", p.Number, p.ref)
		merged[key] = append(merged[key], struct{ seq int; msg string }{p.seq, p.Message})
	}

	for key, fragments := range merged {
		sort.Slice(fragments, func(i, j int) bool { return fragments[i].seq < fragments[j].seq })
		fullMsg := ""
		for _, f := range fragments { fullMsg += f.msg }

		// 从部分中查找原始元数据（效率低但简单）
		for _, p := range parts {
			if fmt.Sprintf("%s_%d", p.Number, p.ref) == key && p.seq == 1 {
				p.SMS.Message = fullMsg
				result = append(result, p.SMS)
				break
			}
		}
	}

	sort.Slice(result, func(i, j int) bool { return result[i].Index < result[j].Index })
	return result, nil
}

// SendSMS 发送短信。
func (s *SerialService) SendSMS(number, message string) error {
	pdu, length, err := encodePDU(number, message)
	if err != nil {
		return err
	}

	if _, err := s.SendATCommand(fmt.Sprintf(cmdSendSMS, length)); err != nil {
		return err
	}
	_, err = s.sendRawCommand(pdu, ctrlZ, 60*time.Second)
	return err
}

// DeleteSMS 删除指定索引的短信。
func (s *SerialService) DeleteSMS(index int) error {
	_, err := s.SendATCommand(fmt.Sprintf(cmdDeleteSMS, index))
	return err
}

// 辅助函数

func extractValue(response string) string {
	for _, line := range strings.Split(response, "\n") {
		line = strings.TrimSpace(line)
		if line != "" && line != respOK && !strings.HasPrefix(line, cmdCheck) {
			return line
		}
	}
	return ""
}

func extractOperator(response string) string {
	if m := reOperator.FindStringSubmatch(response); len(m) > 1 {
		return m[1]
	}
	return ""
}

func getPDUStatus(stat int) string {
	switch stat {
	case 0:
		return "REC UNREAD"
	case 1:
		return "REC READ"
	case 2:
		return "STO UNSENT"
	case 3:
		return "STO SENT"
	default:
		return "UNKNOWN"
	}
}

