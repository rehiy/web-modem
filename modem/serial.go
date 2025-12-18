package modem

import (
	"encoding/hex"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf16"

	"github.com/tarm/serial"
)

const (
	// AT 命令
	cmdEchoOff      = "ATE0"
	cmdTextMode     = "AT+CMGF=1"
	cmdCheck        = "AT"
	cmdListSMS      = "AT+CMGL=\"ALL\""
	cmdDeleteSMS    = "AT+CMGD=%d"
	cmdSendSMS      = "AT+CMGS=\"%s\""
	cmdSignal       = "AT+CSQ"
	cmdManufacturer = "AT+CGMI"
	cmdModel        = "AT+CGMM"
	cmdIMEI         = "AT+CGSN"
	cmdIMSI         = "AT+CIMI"
	cmdOperator     = "AT+COPS?"
	cmdNumber       = "AT+CNUM"
	
	// 超时和延迟
	readTimeout     = 100 * time.Millisecond
	errorSleep      = 100 * time.Millisecond
	bufferSize      = 128
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
	if !strings.Contains(resp, "OK") {
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
	return s.sendRawCommand(command, "\r\n")
}

// sendRawCommand 发送原始命令并读取响应。
func (s *SerialService) sendRawCommand(command, suffix string) (string, error) {
	s.Lock()
	defer s.Unlock()

	_ = s.port.Flush()
	if _, err := s.port.Write([]byte(command + suffix)); err != nil {
		return "", err
	}

	var resp strings.Builder
	buf := make([]byte, bufferSize)
	
	for {
		n, err := s.port.Read(buf)
		if n > 0 {
			resp.Write(buf[:n])
			str := resp.String()
			if strings.Contains(str, "OK") || strings.Contains(str, "ERROR") || strings.Contains(str, ">") {
				return str, nil
			}
		}
		if err != nil {
			if resp.Len() > 0 { return resp.String(), nil }
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
	if m := regexp.MustCompile(`\+CNUM:.*,"([^"]+)"`).FindStringSubmatch(resp); len(m) > 1 {
		return m[1], nil
	}
	return "", errors.New("not found")
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
	
	// 按 +CMGL: 分割以处理多条消息
	chunks := strings.Split(resp, "+CMGL: ")
	for _, chunk := range chunks[1:] { // 跳过第一个空部分
		lines := strings.SplitN(chunk, "\n", 2)
		if len(lines) < 2 { continue }
		
		meta, content := lines[0], strings.TrimSpace(strings.TrimSuffix(lines[1], "OK"))
		// 解析元数据: index,"status","oa",,"scts"
		fields := strings.Split(meta, ",")
		if len(fields) < 5 { continue }
		
		idx, _ := strconv.Atoi(strings.TrimSpace(fields[0]))
		txt, ref, tot, seq := decodeHexSMS(content)
		
		parts = append(parts, struct{ SMS; ref, total, seq int }{
			SMS: SMS{
				Index:   idx,
				Status:  strings.Trim(fields[1], `"`),
				Number:  strings.Trim(fields[2], `"`),
				Time:    strings.Trim(fields[4], `"`),
				Message: txt,
			},
			ref: ref, total: tot, seq: seq,
		})
	}

	// 合并长短信
	merged := make(map[string][]struct{ seq int; msg string })
	var result []SMS
	
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
	if _, err := s.SendATCommand(fmt.Sprintf(cmdSendSMS, number)); err != nil {
		return err
	}
	_, err := s.sendRawCommand(message, "\x1A") // \x1A 是 Ctrl+Z
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
		if line != "" && line != "OK" && !strings.HasPrefix(line, "AT") {
			return line
		}
	}
	return ""
}

func extractOperator(response string) string {
	if m := regexp.MustCompile(`"([^"]+)"`).FindStringSubmatch(response); len(m) > 1 {
		return m[1]
	}
	return ""
}

func decodeHexSMS(content string) (string, int, int, int) {
	content = strings.TrimSpace(content)
	b, err := hex.DecodeString(content)
	if err != nil || len(content)%2 != 0 { return content, 0, 1, 1 }

	offset, ref, total, seq := 0, 0, 1, 1
	
	// 检查级联短信 UDH（用户数据头）
	// 05 00 03 [引用] [总数] [序号]
	if len(b) > 6 && b[0] == 5 && b[1] == 0 && b[2] == 3 {
		offset, ref, total, seq = 6, int(b[3]), int(b[4]), int(b[5])
	} else if len(b) > 7 && b[0] == 6 && b[1] == 8 && b[2] == 4 {
		// 06 08 04 [引用1] [引用2] [总数] [序号]
		offset, ref, total, seq = 7, int(b[3])<<8|int(b[4]), int(b[5]), int(b[6])
	}

	if (len(b)-offset)%2 != 0 { return content, 0, 1, 1 }
	
	// 解码 UTF-16BE
	u16 := make([]uint16, (len(b)-offset)/2)
	for i := range u16 {
		u16[i] = uint16(b[offset+i*2])<<8 | uint16(b[offset+i*2+1])
	}
	return string(utf16.Decode(u16)), ref, total, seq
}
