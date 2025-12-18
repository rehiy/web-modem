package services

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/tarm/serial"

	"modem-manager/models"
)

var (
	managerInstance *SerialManager
	managerOnce     sync.Once
)

// SerialManager 负责管理不同串口对应的 SerialService 实例。
type SerialManager struct {
	services map[string]*SerialService
	mu       sync.Mutex
}

// GetSerialManager 返回全局串口管理器。
func GetSerialManager() *SerialManager {
	managerOnce.Do(func() {
		managerInstance = &SerialManager{services: make(map[string]*SerialService)}
	})
	return managerInstance
}

// ScanAndConnectAll 扫描并连接支持 AT 的 modem，返回已连接端口列表。
func (m *SerialManager) ScanAndConnectAll(baudRate int) ([]models.SerialPort, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	usbPorts, _ := filepath.Glob("/dev/ttyUSB*")
	acmPorts, _ := filepath.Glob("/dev/ttyACM*")
	candidates := append(usbPorts, acmPorts...)

	for _, p := range candidates {
		if _, ok := m.services[p]; ok {
			continue // 已连接
		}

		cfg := &serial.Config{Name: p, Baud: baudRate, ReadTimeout: 2 * time.Second, Size: 8, Parity: serial.ParityNone, StopBits: serial.Stop1}
		sp, err := serial.OpenPort(cfg)
		if err != nil {
			continue
		}

		if _, err := sp.Write([]byte("AT\r\n")); err != nil {
			_ = sp.Close()
			continue
		}
		buf := make([]byte, 128)
		resp := ""
		deadline := time.Now().Add(1 * time.Second)
		for time.Now().Before(deadline) {
			n, _ := sp.Read(buf)
			if n > 0 {
				resp += string(buf[:n])
				if strings.Contains(resp, "OK") {
					break
				}
			}
			time.Sleep(30 * time.Millisecond)
		}
		if !strings.Contains(resp, "OK") {
			_ = sp.Close()
			continue
		}

		service := newSerialService(p, sp)
		m.services[p] = service
		go service.readLoop()

		sp.Write([]byte("ATE0\r\n"))
		time.Sleep(100 * time.Millisecond)
		sp.Write([]byte("AT+CMGF=0\r\n"))
	}

	names := make([]string, 0, len(m.services))
	for name := range m.services {
		names = append(names, name)
	}
	sort.Strings(names)

	var result []models.SerialPort
	for _, name := range names {
		result = append(result, models.SerialPort{Name: name, Path: name, Connected: true})
	}
	return result, nil
}

// GetService 根据端口名返回对应的 SerialService。
func (m *SerialManager) GetService(name string) (*SerialService, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	service, ok := m.services[name]
	if !ok {
		return nil, fmt.Errorf("port not connected: %s", name)
	}
	return service, nil
}
