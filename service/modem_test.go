package service

import (
	"errors"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/rehiy/modem/at"
)

// scriptedPort 是按指令返回预设响应的假串口，用于驱动 at.Device。
type scriptedPort struct {
	reader    *io.PipeReader
	writer    *io.PipeWriter
	responses map[string][]string
	mu        sync.Mutex
	sent      []string
}

func newScriptedPort(responses map[string][]string) *scriptedPort {
	r, w := io.Pipe()
	return &scriptedPort{reader: r, writer: w, responses: responses}
}

func (p *scriptedPort) Read(buf []byte) (int, error) { return p.reader.Read(buf) }

func (p *scriptedPort) Write(data []byte) (int, error) {
	cmd := strings.TrimSpace(string(data))
	p.mu.Lock()
	p.sent = append(p.sent, cmd)
	p.mu.Unlock()
	lines, ok := p.responses[cmd]
	if !ok {
		lines = []string{"OK"}
	}
	go p.writer.Write([]byte(strings.Join(lines, "\r\n") + "\r\n"))
	return len(data), nil
}

func (p *scriptedPort) Flush() error { return nil }

func (p *scriptedPort) Close() error {
	p.writer.Close()
	return p.reader.Close()
}

func (p *scriptedPort) count(cmd string) int {
	p.mu.Lock()
	defer p.mu.Unlock()
	n := 0
	for _, c := range p.sent {
		if c == cmd {
			n++
		}
	}
	return n
}

func newScriptedConn(t *testing.T, responses map[string][]string) (*ModemConn, *scriptedPort) {
	t.Helper()
	port := newScriptedPort(responses)
	dev := at.New(port, nil, &at.Config{Timeout: time.Second, Printf: func(string, ...any) {}})
	t.Cleanup(func() { dev.Close() })
	return &ModemConn{Name: "ttyTEST", state: ModemState{Name: "ttyTEST", Connected: true}, Device: dev}, port
}

func newTestModemService() *ModemService {
	return &ModemService{
		pool:        map[string]*ModemConn{},
		urcWaiters:  map[string]map[int]chan ModemURC{},
		failedPorts: map[string]time.Time{},
		euiccIMEIs:  map[string]bool{},
		infoCache:   NewTTLCache[map[string]any](time.Minute),
		signalCache: NewTTLCache[map[string]any](time.Minute),
	}
}

// drainModemEvents 清空全局事件队列，返回其中的设备列表变化事件数。
func drainModemEvents() int {
	n := 0
	for {
		select {
		case ev := <-ModemEvent:
			if ev.Event == ModemEventModems {
				n++
			}
		default:
			return n
		}
	}
}

func TestSendOptionalCommandRemembersUnsupported(t *testing.T) {
	conn, port := newScriptedConn(t, map[string][]string{
		"AT+QENG=\"servingcell\"": {"ERROR"},
		"AT+CESQ":                 {"+CESQ: 99,99,255,255,20,50", "OK"},
	})

	for i := 0; i < 3; i++ {
		if _, err := conn.SendOptionalCommand(`AT+QENG="servingcell"`); !errors.Is(err, ErrCommandUnsupported) {
			t.Fatalf("第 %d 次: err = %v, want ErrCommandUnsupported", i, err)
		}
	}
	if n := port.count(`AT+QENG="servingcell"`); n != 1 {
		t.Fatalf("不支持的指令发送了 %d 次, want 1", n)
	}

	for i := 0; i < 2; i++ {
		if _, err := conn.SendOptionalCommand("AT+CESQ"); err != nil {
			t.Fatalf("支持的指令返回错误: %v", err)
		}
	}
	if n := port.count("AT+CESQ"); n != 2 {
		t.Fatalf("支持的指令应每次发送, got %d", n)
	}
}

func TestListModemStatesSnapshot(t *testing.T) {
	m := newTestModemService()
	m.pool["ttyUSB2"] = &ModemConn{Name: "ttyUSB2", state: ModemState{Name: "ttyUSB2", Connected: true}}
	m.pool["ttyUSB0"] = &ModemConn{Name: "ttyUSB0", state: ModemState{Name: "ttyUSB0", Connected: true}}
	m.pool["ttyUSB1"] = &ModemConn{Name: "ttyUSB1", state: ModemState{Name: "ttyUSB1"}}

	online := m.ListModemStates(true)
	if len(online) != 2 || online[0].Name != "ttyUSB0" || online[1].Name != "ttyUSB2" {
		t.Fatalf("在线快照 = %+v, want 按名称排序的 ttyUSB0, ttyUSB2", online)
	}
	if all := m.ListModemStates(false); len(all) != 3 {
		t.Fatalf("全部快照数量 = %d, want 3", len(all))
	}
}

func TestSetEuiccDetectedRemembersByIMEI(t *testing.T) {
	drainModemEvents()
	m := newTestModemService()
	conn := &ModemConn{Name: "ttyUSB2", state: ModemState{Name: "ttyUSB2", IMEI: "861234567890123"}}

	m.SetEuiccDetected(conn, true)
	if !conn.State().HasEuicc {
		t.Fatal("HasEuicc 未置位")
	}
	if !m.knownEuicc("861234567890123") {
		t.Fatal("未按 IMEI 记忆 eUICC 判定")
	}
	if n := drainModemEvents(); n != 1 {
		t.Fatalf("状态变化应推送 1 次设备列表事件, got %d", n)
	}

	m.SetEuiccDetected(conn, true)
	if n := drainModemEvents(); n != 0 {
		t.Fatalf("状态未变化不应推送事件, got %d", n)
	}
}

func TestInvalidateModemCache(t *testing.T) {
	m := newTestModemService()
	load := func() (map[string]any, error) { return map[string]any{"ok": true}, nil }
	m.LoadModemInfo("/dev/ttyUSB2", false, load)
	m.LoadModemSignal("ttyUSB2", false, load)

	m.InvalidateModemCache("ttyUSB2")
	if _, ok := m.infoCache.Get("ttyUSB2"); ok {
		t.Fatal("信息缓存未失效")
	}
	if _, ok := m.signalCache.Get("ttyUSB2"); ok {
		t.Fatal("信号缓存未失效")
	}
}

func TestMarkMissingDisconnected(t *testing.T) {
	m := newTestModemService()
	conn, _ := newScriptedConn(t, nil)
	conn.Name = "ttyUSB2"
	m.pool["ttyUSB2"] = conn
	m.infoCache.Set("ttyUSB2", map[string]any{})

	if changed := m.markMissingDisconnected([]string{"/dev/ttyUSB0"}); !changed {
		t.Fatal("端口消失应返回状态变化")
	}
	if conn.State().Connected {
		t.Fatal("消失的端口应标记为断开")
	}
	if _, ok := m.infoCache.Get("ttyUSB2"); ok {
		t.Fatal("断开设备的信息缓存应失效")
	}
	if changed := m.markMissingDisconnected([]string{"/dev/ttyUSB0"}); changed {
		t.Fatal("已断开的端口再次检查不应视为变化")
	}
}

// 回归：端口正被长操作占用时，makeConnect 的在线检测不得持有全局锁，
// 否则所有接口的 GetConn 都会被卡住。
func TestMakeConnectDoesNotBlockGetConnWhilePortBusy(t *testing.T) {
	m := newTestModemService()
	conn, _ := newScriptedConn(t, nil)
	m.pool[conn.Name] = conn

	unlock := conn.LockOp() // 模拟 eSIM 会话等长操作占用端口
	connectDone := make(chan struct{})
	go func() {
		m.makeConnect("/dev/" + conn.Name)
		close(connectDone)
	}()
	time.Sleep(50 * time.Millisecond)

	getDone := make(chan struct{})
	go func() {
		m.GetConn(conn.Name)
		close(getDone)
	}()
	select {
	case <-getDone:
	case <-time.After(time.Second):
		unlock()
		t.Fatal("端口忙时 GetConn 被 makeConnect 阻塞")
	}

	unlock()
	select {
	case <-connectDone:
	case <-time.After(3 * time.Second):
		t.Fatal("释放端口后 makeConnect 未完成")
	}
	if !conn.State().Connected {
		t.Fatal("可响应的端口应保持在线")
	}
}
