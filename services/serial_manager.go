package services

import (
	"fmt"
	"path/filepath"
	"sync"

	"modem-manager/models"
)

var (
	managerOnce     sync.Once
	managerInstance *SerialManager
)

type SerialManager struct {
	services map[string]*SerialService
	mu       sync.Mutex
}

// GetSerialManager 返回全局串口管理器。
func GetSerialManager() *SerialManager {
	managerOnce.Do(func() {
		managerInstance = &SerialManager{
			services: make(map[string]*SerialService),
		}
	})
	return managerInstance
}

// Scan 扫描并连接支持 AT 的 modem，返回已连接端口列表。
func (m *SerialManager) Scan(baudRate int) ([]models.SerialPort, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	usbPorts, _ := filepath.Glob("/dev/ttyUSB*")
	acmPorts, _ := filepath.Glob("/dev/ttyACM*")
	candidates := append(usbPorts, acmPorts...)

	for _, p := range candidates {
		if _, ok := m.services[p]; ok {
			continue // 已连接
		}
		if svc, err := NewSerialService(p, baudRate); err == nil {
			m.services[p] = svc
			svc.Start()
		}
	}

	var result []models.SerialPort
	for name := range m.services {
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
