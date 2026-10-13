package service

import (
	"sync"
	"time"
)

// TTLCache 是带过期时间的并发安全键值缓存。
//
// GetOrLoad 对同一 key 的并发加载做合并：加载期间到达的请求（包括强制刷新）
// 直接等待并共享这次加载结果，避免多个请求在同一串口上重复排队执行 AT 指令。
// 加载失败的结果不缓存。
type TTLCache[V any] struct {
	ttl      time.Duration
	now      func() time.Time
	mu       sync.Mutex
	items    map[string]ttlEntry[V]
	inflight map[string]*ttlLoad[V]
}

type ttlEntry[V any] struct {
	value     V
	expiresAt time.Time
}

type ttlLoad[V any] struct {
	done  chan struct{}
	value V
	err   error
}

// NewTTLCache 创建缓存，ttl 为条目有效期。
func NewTTLCache[V any](ttl time.Duration) *TTLCache[V] {
	return &TTLCache[V]{
		ttl:      ttl,
		now:      time.Now,
		items:    map[string]ttlEntry[V]{},
		inflight: map[string]*ttlLoad[V]{},
	}
}

// Get 返回未过期的缓存值。
func (c *TTLCache[V]) Get(key string) (V, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.getLocked(key)
}

func (c *TTLCache[V]) getLocked(key string) (V, bool) {
	entry, ok := c.items[key]
	if !ok || !c.now().Before(entry.expiresAt) {
		var zero V
		return zero, false
	}
	return entry.value, true
}

// Set 写入缓存值。
func (c *TTLCache[V]) Set(key string, value V) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.items[key] = ttlEntry[V]{value: value, expiresAt: c.now().Add(c.ttl)}
}

// Delete 删除缓存值；进行中的加载不受影响，但其结果不会写回缓存。
func (c *TTLCache[V]) Delete(key string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.items, key)
	delete(c.inflight, key)
}

// GetOrLoad 命中且未要求强制刷新时直接返回缓存；否则调用 load 加载并缓存。
func (c *TTLCache[V]) GetOrLoad(key string, force bool, load func() (V, error)) (V, error) {
	c.mu.Lock()
	if !force {
		if value, ok := c.getLocked(key); ok {
			c.mu.Unlock()
			return value, nil
		}
	}
	if call, ok := c.inflight[key]; ok {
		c.mu.Unlock()
		<-call.done
		return call.value, call.err
	}
	call := &ttlLoad[V]{done: make(chan struct{})}
	c.inflight[key] = call
	c.mu.Unlock()

	call.value, call.err = load()

	c.mu.Lock()
	// 加载期间被 Delete 失效时，结果可能已过时，只返回给本次调用方，不写回缓存
	if c.inflight[key] == call {
		delete(c.inflight, key)
		if call.err == nil {
			c.items[key] = ttlEntry[V]{value: call.value, expiresAt: c.now().Add(c.ttl)}
		}
	}
	c.mu.Unlock()
	close(call.done)
	return call.value, call.err
}
