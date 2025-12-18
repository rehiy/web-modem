package services

import (
	"fmt"
	"path/filepath"
	"sync"

	"modem-manager/modem"
)

var (
	managerOnce     sync.Once
	managerInstance *SerialManager
)

// SerialManager 管理多个串口连接。
type SerialManager struct {
	pool map[string]*modem.SerialService
	mu   sync.Mutex
}

// GetSerialManager 返回 SerialManager 的单例实例。
func GetSerialManager() *SerialManager {
	managerOnce.Do(func() {
		managerInstance = &SerialManager{
			pool: make(map[string]*modem.SerialService),
		}
	})
	return managerInstance
}

// Scan 扫描可用的调制解调器并连接到它们。
// 它查找匹配 /dev/ttyUSB* 和 /dev/ttyACM* 的设备。
func (m *SerialManager) Scan(baudRate int) ([]modem.SerialPort, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// 查找潜在设备
	usb, _ := filepath.Glob("/dev/ttyUSB*")
	acm, _ := filepath.Glob("/dev/ttyACM*")
	
	// 尝试连接到新设备
	for _, p := range append(usb, acm...) {
		if _, exists := m.pool[p]; !exists {
			broadcast := GetEventListener().Broadcast
			if svc, err := modem.NewSerialService(p, baudRate, broadcast); err == nil {
				m.pool[p] = svc
				svc.Start()
			}
		}
	}

	// 从活动连接构建结果列表
	var result []modem.SerialPort
	for name := range m.pool {
		result = append(result, modem.SerialPort{
			Name:      name,
			Path:      name,
			Connected: true,
		})
	}
	return result, nil
}

// GetService 返回给定端口名称的 SerialService。
func (m *SerialManager) GetService(name string) (*modem.SerialService, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	service, ok := m.pool[name]
	if !ok {
		return nil, fmt.Errorf("port not connected: %s", name)
	}
	return service, nil
}
