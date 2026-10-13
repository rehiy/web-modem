package service

import (
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func newTestCache(ttl time.Duration) (*TTLCache[int], *time.Time) {
	c := NewTTLCache[int](ttl)
	now := time.Unix(1000, 0)
	c.now = func() time.Time { return now }
	return c, &now
}

func TestTTLCacheExpires(t *testing.T) {
	c, now := newTestCache(10 * time.Second)
	c.Set("a", 1)
	if v, ok := c.Get("a"); !ok || v != 1 {
		t.Fatalf("Get = %v, %v; want 1, true", v, ok)
	}
	*now = now.Add(10 * time.Second)
	if _, ok := c.Get("a"); ok {
		t.Fatal("过期条目仍被命中")
	}
}

func TestTTLCacheGetOrLoad(t *testing.T) {
	c, _ := newTestCache(time.Minute)
	calls := 0
	load := func() (int, error) { calls++; return calls, nil }

	if v, _ := c.GetOrLoad("a", false, load); v != 1 {
		t.Fatalf("首次加载 = %d, want 1", v)
	}
	if v, _ := c.GetOrLoad("a", false, load); v != 1 || calls != 1 {
		t.Fatalf("命中缓存 = %d (calls=%d), want 1 (calls=1)", v, calls)
	}
	if v, _ := c.GetOrLoad("a", true, load); v != 2 {
		t.Fatalf("强制刷新 = %d, want 2", v)
	}
	c.Delete("a")
	if v, _ := c.GetOrLoad("a", false, load); v != 3 {
		t.Fatalf("失效后加载 = %d, want 3", v)
	}
}

func TestTTLCacheErrorNotCached(t *testing.T) {
	c, _ := newTestCache(time.Minute)
	if _, err := c.GetOrLoad("a", false, func() (int, error) { return 0, errors.New("boom") }); err == nil {
		t.Fatal("应返回加载错误")
	}
	if _, ok := c.Get("a"); ok {
		t.Fatal("加载失败的结果不应缓存")
	}
}

func TestTTLCacheConcurrentLoadsShareResult(t *testing.T) {
	c, _ := newTestCache(time.Minute)
	var calls atomic.Int32
	release := make(chan struct{})
	load := func() (int, error) {
		calls.Add(1)
		<-release
		return 42, nil
	}

	var wg sync.WaitGroup
	results := make([]int, 5)
	for i := range results {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			// 强制刷新也应合并到进行中的加载
			results[i], _ = c.GetOrLoad("a", i%2 == 0, load)
		}(i)
	}
	// 等所有协程进入等待后再放行
	for {
		c.mu.Lock()
		_, loading := c.inflight["a"]
		c.mu.Unlock()
		if loading {
			break
		}
		time.Sleep(time.Millisecond)
	}
	time.Sleep(20 * time.Millisecond)
	close(release)
	wg.Wait()

	if n := calls.Load(); n != 1 {
		t.Fatalf("并发加载次数 = %d, want 1", n)
	}
	for i, v := range results {
		if v != 42 {
			t.Fatalf("results[%d] = %d, want 42", i, v)
		}
	}
}

func TestTTLCacheDeleteDuringLoadDiscardsResult(t *testing.T) {
	c, _ := newTestCache(time.Minute)
	v, _ := c.GetOrLoad("a", false, func() (int, error) {
		c.Delete("a") // 模拟加载期间发生写操作使缓存失效
		return 7, nil
	})
	if v != 7 {
		t.Fatalf("调用方应拿到本次加载结果, got %d", v)
	}
	if _, ok := c.Get("a"); ok {
		t.Fatal("加载期间被失效的结果不应写回缓存")
	}
}
