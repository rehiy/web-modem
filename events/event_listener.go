package events

import "sync"

var (
	listenerOnce     sync.Once
	listenerInstance *EventListener
)

// EventListener 管理事件订阅和广播。
type EventListener struct {
	pool map[chan string]struct{}
	sync.RWMutex
}

// GetEventListener 返回 EventListener 的单例实例。
func GetEventListener() *EventListener {
	listenerOnce.Do(func() {
		listenerInstance = &EventListener{pool: make(map[chan string]struct{})}
	})
	return listenerInstance
}

// Broadcast 非阻塞地向所有订阅者发送消息。
// 如果订阅者的通道已满，则跳过该订阅者的消息。
func (el *EventListener) Broadcast(msg string) {
	el.RLock()
	defer el.RUnlock()

	for ch := range el.pool {
		select {
		case ch <- msg:
		default:
			// 通道已满，跳过消息
		}
	}
}

// Subscribe 创建一个新的订阅通道。
// 返回接收消息的通道和取消订阅的函数。
func (el *EventListener) Subscribe(buffer int) (chan string, func()) {
	if buffer <= 0 {
		buffer = 100
	}
	ch := make(chan string, buffer)

	el.Lock()
	el.pool[ch] = struct{}{}
	el.Unlock()

	return ch, func() {
		el.Lock()
		defer el.Unlock()
		if _, ok := el.pool[ch]; ok {
			delete(el.pool, ch)
			close(ch)
		}
	}
}
