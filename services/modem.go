package services

import (
	"fmt"
	"log"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/rehiy/modem/at"
	"github.com/tarm/serial"
)

var (
	managerOnce     sync.Once
	managerInstance *ModemManager

	// 用于推送串口事件到客户端
	EventChannel = make(chan string, 100)
)

// ModemInfo 端口信息
type ModemInfo struct {
	Name      string `json:"name"`
	Connected bool   `json:"connected"`
}

// ModemManager 管理多个串口连接
type ModemManager struct {
	pool map[string]*at.Device
	mu   sync.Mutex
}

// GetModemManager 返回单例实例
func GetModemManager() *ModemManager {
	managerOnce.Do(func() {
		managerInstance = &ModemManager{
			pool: map[string]*at.Device{},
		}
	})
	return managerInstance
}

// GetModems 返回已连接的端口信息
func (m *ModemManager) GetModems() []ModemInfo {
	m.mu.Lock()
	defer m.mu.Unlock()

	var modems []ModemInfo
	for name, conn := range m.pool {
		modems = append(modems, ModemInfo{
			Name:      name,
			Connected: conn.IsOpen(),
		})
	}
	return modems
}

// ScanModems 扫描可用的调制解调器并连接到它们
func (m *ModemManager) ScanModems(devs ...string) {
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

// GetConnect 返回给定端口名称的 AT 接口
func (m *ModemManager) GetConnect(u string) (*at.Device, error) {
	n := path.Base(u)

	m.mu.Lock()
	defer m.mu.Unlock()

	conn, ok := m.pool[n]
	if !ok {
		return nil, fmt.Errorf("[%s] not found", n)
	}
	return conn, nil
}

// makeConnect 添加新的 AT 接口
func (m *ModemManager) makeConnect(u string) error {
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
		conn.Close()
		delete(m.pool, n)
	}

	// 创建事件处理函数，写入 EventChannel
	hf := func(l string, p map[int]string) {
		EventChannel <- fmt.Sprintf("[%s] urc:%s %v", n, l, p)
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

	// 创建新的连接
	conn := at.New(port, hf, &at.Config{Printf: pf})
	if err := conn.Test(); err != nil {
		pf("at test failed: %v", err)
		conn.Close()
		return err
	}

	// 设置默认参数
	conn.EchoOff()     // 关闭回显
	conn.SetSMSMode(0) // PDU 模式

	// 添加到连接池
	m.pool[n] = conn
	pf("connected")
	return nil
}
