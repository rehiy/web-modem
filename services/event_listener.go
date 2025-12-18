package services

import "sync"

var (
	listenerOnce     sync.Once
	listenerInstance *EventListener
)

type EventListener struct {
	pool map[chan string]struct{}
	mu        sync.RWMutex
}

// GetEventListener 返回全局监听管理器。
func GetEventListener() *EventListener {
	listenerOnce.Do(func() {
		listenerInstance = &EventListener{
			pool: make(map[chan string]struct{}),
		}
	})
	return listenerInstance
}

// Broadcast 向所有监听器发送消息（非阻塞，包内使用）。
func (el *EventListener) Broadcast(message string) {
	el.mu.RLock()
	snapshot := make([]chan string, 0, len(el.pool))
	for ch := range el.pool {
		snapshot = append(snapshot, ch)
	}
	el.mu.RUnlock()

	for _, listener := range snapshot {
		select {
		case listener <- message:
		default:
		}
	}
}

// Subscribe 便捷订阅：返回通道与取消函数。
func (el *EventListener) Subscribe(buffer int) (chan string, func()) {
	if buffer <= 0 {
		buffer = 100
	}
	ch := make(chan string, buffer)
	el.addListener(ch)
	cancel := func() { el.removeListener(ch) }
	return ch, cancel
}

// addListener 注册一个新的监听通道（包内使用）。
func (el *EventListener) addListener(ch chan string) {
	el.mu.Lock()
	el.pool[ch] = struct{}{}
	el.mu.Unlock()
}

// removeListener 注销监听通道并关闭它（包内使用）。
func (el *EventListener) removeListener(ch chan string) {
	el.mu.Lock()
	if _, ok := el.pool[ch]; ok {
		delete(el.pool, ch)
		close(ch)
	}
	el.mu.Unlock()
}
