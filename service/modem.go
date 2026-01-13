package service

import (
	"fmt"
	"log"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/rehiy/modem/at"
	"github.com/tarm/serial"
)

var (
	modemOnce     sync.Once
	modemInstance *ModemService
	ModemEvent    = make(chan string, 100)
)

// ModemConn 端口连接
type ModemConn struct {
	Name       string `json:"name"`
	Number     string `json:"number"`
	Connected  bool   `json:"connected"`
	*at.Device `json:"-"`
}

// ModemService 管理多个串口连接
type ModemService struct {
	pool map[string]*ModemConn
	mu   sync.Mutex
}

// GetModemService 返回单例实例
func GetModemService() *ModemService {
	modemOnce.Do(func() {
		modemInstance = &ModemService{
			pool: map[string]*ModemConn{},
		}
	})
	return modemInstance
}

// ScanModems 扫描可用的调制解调器并连接到它们
func (m *ModemService) ScanModems(devs ...string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// 环境变量
	if len(devs) == 0 {
		port := os.Getenv("MODEM_PORT")
		if port != "" {
			devs = strings.Split(port, ",")
		}
	}

	// 查找潜在设备
	switch runtime.GOOS {
	case "windows":
		if len(devs) == 0 {
			devs = []string{"COM1", "COM2", "COM3", "COM4", "COM5"}
		}
	default:
		if len(devs) == 0 {
			devs = []string{"/dev/ttyUSB*", "/dev/ttyACM*"}
		}
		pps := []string{}
		for _, p := range devs {
			matches, _ := filepath.Glob(p)
			pps = append(pps, matches...)
		}
		devs = pps
	}

	// 尝试连接到新设备
	for _, u := range devs {
		m.makeConnect(u)
	}
}

// GetConnList 返回已连接的端口信息
func (m *ModemService) GetConnList() []*ModemConn {
	m.mu.Lock()
	defer m.mu.Unlock()

	var conns []*ModemConn
	for _, model := range m.pool {
		conns = append(conns, model)
	}
	return conns
}

// GetConn 返回给定端口名称的 AT 接口
func (m *ModemService) GetConn(u string) (*ModemConn, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	n := path.Base(u)
	conn, ok := m.pool[n]
	if !ok {
		return nil, fmt.Errorf("[%s] not found", n)
	}
	if !conn.Connected || conn.Device == nil || conn.Device.IsOpen() == false {
		return nil, fmt.Errorf("[%s] not connected", n)
	}
	return conn, nil
}

// handleIncomingSMS 处理指定端口的新接收短信
func (m *ModemService) handleIncomingSMS(portName string, smsIndex int) {
	conn, err := m.GetConn(portName)
	if err != nil {
		log.Printf("[%s] Failed to get connection for incoming SMS: %v", portName, err)
		return
	}

	// 获取短信列表（只获取新短信）
	smsList, err := conn.ListSMSPdu(4)
	if err != nil {
		log.Printf("[%s] Failed to list SMS: %v", portName, err)
		return
	}

	smsdbService := NewSmsdbService()
	webhookService := NewWebhookService()

	// 处理每条短信
	for _, sms := range smsList {
		hasNewSMS := false
		for _, idx := range sms.Indices {
			if idx == smsIndex {
				hasNewSMS = true
				break
			}
		}
		if hasNewSMS {
			log.Printf("[%s] New SMS from %s: %s", portName, sms.Number, sms.Text)
			modelSMS := atSMSToModelSMS(sms, conn.Number, conn.Name)
			smsdbService.HandleIncomingSMS(modelSMS)
			webhookService.HandleIncomingSMS(modelSMS)
		}
	}
}

// makeConnect 添加新的 AT 接口
func (m *ModemService) makeConnect(u string) error {
	n := path.Base(u)

	// 创建日志函数
	pf := func(s string, v ...any) {
		log.Printf(fmt.Sprintf("[%s] %s", n, s), v...)
	}

	// 检查是否已连接
	if conn, ok := m.pool[n]; ok {
		if conn.Test() == nil {
			pf("already connected")
			return nil
		}
		conn.Connected = false
		conn.Close()
	}

	// 创建事件处理函数，写入 ModemEvent 并处理短信
	hf := func(e string, p map[int]string) {
		ModemEvent <- fmt.Sprintf("%s, %s, %v", n, e, p)
		// 处理收到的短信通知
		if e == "+CMTI" && len(p) > 0 {
			if indexStr, ok := p[1]; ok {
				if index, err := strconv.Atoi(indexStr); err == nil {
					m.handleIncomingSMS(n, index)
				}
			}
		}
	}

	// 打开串口
	pf("connecting")
	port, err := serial.OpenPort(&serial.Config{
		Name:        u,      // 串口完整路径
		Baud:        115200, // 波特率
		ReadTimeout: 1 * time.Second,
	})
	if err != nil {
		pf("connect failed: %v", err)
		return err
	}

	// 链接新设备
	modem := at.New(port, hf, &at.Config{Printf: pf})
	if err := modem.Test(); err != nil {
		pf("at test failed: %v", err)
		modem.Close()
		return err
	}

	// 设置默认参数
	modem.EchoOff()     // 关闭回显
	modem.SetSMSMode(0) // PDU 模式

	// 添加到连接池
	m.pool[n] = &ModemConn{
		Name:      n,
		Number:    "unkown",
		Connected: true,
		Device:    modem,
	}

	// 获取手机号，用于接收号码
	if number, _, err := modem.GetNumber(); err == nil {
		pf("connected, phone number: %s", number)
		m.pool[n].Number = number
	} else {
		pf("connected, but failed to get phone number: %v", err)
	}

	return nil
}
