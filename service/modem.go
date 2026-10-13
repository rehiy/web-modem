package service

import (
	"fmt"
	"log"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/rehiy/modem/at"
	"go.bug.st/serial"
)

// ModemEventMsg 是推送给 SSE 客户端的设备事件。
type ModemEventMsg struct {
	Event string // SSE 事件名，见 ModemEventURC / ModemEventModems
	Data  string
}

const (
	// ModemEventURC 为模组上报的 URC 原文，前端写入实时日志
	ModemEventURC = "message"
	// ModemEventModems 表示设备列表（上线/掉线/eUICC 判定）发生变化，前端据此重新读取列表快照
	ModemEventModems = "modems"
)

var (
	modemOnce     sync.Once
	modemInstance *ModemService
	ModemEvent    = make(chan ModemEventMsg, 100)
)

// ModemState 是端口连接的可观测状态，由 ModemConn.stateMu 保护，对外只输出副本。
type ModemState struct {
	Name         string `json:"name"`
	Number       string `json:"number"`
	IMEI         string `json:"imei"`
	ICCID        string `json:"iccid,omitempty"`
	IMSI         string `json:"imsi,omitempty"`
	Operator     string `json:"operator,omitempty"`
	Manufacturer string `json:"manufacturer,omitempty"`
	Model        string `json:"model,omitempty"`
	Revision     string `json:"revision,omitempty"`
	Connected    bool   `json:"connected"`
	HasEuicc     bool   `json:"hasEuicc"` // 该端口的 SIM 是否为 eUICC（读取 eSIM 时惰性判定）
}

// ModemConn 端口连接。Name/Path 创建后不变；其余可变状态经 State()/Set* 访问。
type ModemConn struct {
	Name        string
	Path        string
	stateMu     sync.RWMutex
	state       ModemState
	unsupported map[string]struct{} // 已确认不支持的可选指令，由 stateMu 保护
	opMu        sync.Mutex
	smsMu       sync.Mutex // 串行化收信处理（+CMTI 事件与兜底轮询），避免同一条短信被重复入库/触发 webhook
	*at.Device
}

type serialPort struct {
	serial.Port
}

func (p *serialPort) Flush() error {
	if err := p.ResetInputBuffer(); err != nil {
		return err
	}
	return p.ResetOutputBuffer()
}

// ModemService 管理多个串口连接。
//
// 连接池即设备列表快照：后台监视（StartModemWatch）定期扫描刷新，
// 前端读取列表直接返回快照，不再每次现场扫描串口。
type ModemService struct {
	pool         map[string]*ModemConn
	mu           sync.Mutex
	scanMu       sync.Mutex // 串行化扫描，避免并发扫描重复打开同一端口
	urcMu        sync.Mutex
	urcWaiters   map[string]map[int]chan ModemURC
	nextURCID    int
	failedPorts  map[string]time.Time // 连接失败端口路径 -> 最近失败时间，冷却期内跳过
	lastScan     time.Time            // 最近一次默认扫描开始时间，用于节流
	lastScanDone time.Time            // 最近一次默认扫描完成时间，零值表示尚未完成过扫描
	euiccIMEIs   map[string]bool      // IMEI -> 是否 eUICC，端口重新枚举/重连后恢复判定
	infoCache    *TTLCache[map[string]any]
	signalCache  *TTLCache[map[string]any]
}

// portCooldown 是连接失败端口的冷却时间，避免反复对非 AT 口（diag/数据口）
// 重试拖慢扫描，也减少对模块的干扰。
const portCooldown = 2 * time.Minute

// scanInterval 是默认（非显式指定端口）扫描的最小间隔，避免频繁全量扫描干扰模块。
const scanInterval = 3 * time.Second

// modemRefreshInterval 是后台全量扫描的周期，用于发现新插入的设备、
// 校验已连接端口是否仍可响应，使设备列表快照保持新鲜。
const modemRefreshInterval = 15 * time.Second

// 设备信息/信号缓存有效期。信息中多为 SIM/注册类慢变数据；信号变化较快，
// 有效期更短。写操作（切卡、改号、选网等）会主动失效缓存，前端「刷新」强制重读。
const (
	modemInfoTTL   = 30 * time.Second
	modemSignalTTL = 10 * time.Second
)

// GetModemService 返回单例实例
func GetModemService() *ModemService {
	modemOnce.Do(func() {
		modemInstance = &ModemService{
			pool:        map[string]*ModemConn{},
			urcWaiters:  map[string]map[int]chan ModemURC{},
			failedPorts: map[string]time.Time{},
			euiccIMEIs:  map[string]bool{},
			infoCache:   NewTTLCache[map[string]any](modemInfoTTL),
			signalCache: NewTTLCache[map[string]any](modemSignalTTL),
		}
	})
	return modemInstance
}

// ScanModems 扫描可用的调制解调器并连接到它们。
// 显式传入端口列表时不节流；默认扫描（无参）受 scanInterval 节流。
func (m *ModemService) ScanModems(devs ...string) {
	if len(devs) == 0 && m.scanThrottled() {
		return
	}
	m.scan(devs)
}

// RescanModems 立即执行一次默认全量扫描（忽略节流）。
// 若等待扫描锁期间已有其它扫描完成，则直接复用其结果，不重复扫描。
func (m *ModemService) RescanModems() {
	m.scan(nil)
}

// HasScanned 返回是否已完成过至少一次默认扫描（快照是否可用）。
func (m *ModemService) HasScanned() bool {
	return !m.lastScanCompletedAt().IsZero()
}

func (m *ModemService) lastScanCompletedAt() time.Time {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.lastScanDone
}

func (m *ModemService) scan(devs []string) {
	requestedAt := time.Now()
	m.scanMu.Lock()
	defer m.scanMu.Unlock()

	explicit := len(devs) > 0
	if !explicit && m.lastScanCompletedAt().After(requestedAt) {
		return
	}

	EnsureUSBSerialDrivers()
	devs = resolveModemPorts(devs)

	// 短暂持锁：标记已断开的设备
	m.mu.Lock()
	changed := m.markMissingDisconnected(devs)
	m.mu.Unlock()
	if changed {
		m.notifyModemsChanged()
	}

	// 在锁外逐个连接，避免阻塞其他请求
	for _, u := range devs {
		m.makeConnect(u)
	}

	if !explicit {
		m.mu.Lock()
		m.lastScanDone = time.Now()
		m.mu.Unlock()
	}
}

// resolveModemPorts 将端口列表（可含通配符）展开为实际存在的串口路径；
// 为空时依次取环境变量 MODEM_PORT 与平台默认通配。
func resolveModemPorts(devs []string) []string {
	if len(devs) == 0 {
		if port := os.Getenv("MODEM_PORT"); port != "" {
			devs = strings.Split(port, ",")
		}
	}

	switch runtime.GOOS {
	case "windows":
		if len(devs) == 0 {
			devs = []string{"COM1", "COM2", "COM3", "COM4", "COM5"}
		}
		return devs
	case "darwin":
		if len(devs) == 0 {
			devs = []string{
				"/dev/cu.usbmodem*",
				"/dev/cu.usbserial*",
				"/dev/cu.wchusbserial*",
				"/dev/cu.SLAB_USBtoUART*",
				"/dev/tty.usbmodem*",
				"/dev/tty.usbserial*",
				"/dev/tty.wchusbserial*",
				"/dev/tty.SLAB_USBtoUART*",
			}
		}
	default:
		if len(devs) == 0 {
			devs = []string{"/dev/ttyUSB*", "/dev/ttyACM*"}
		}
	}

	pps := []string{}
	for _, p := range devs {
		matches, _ := filepath.Glob(p)
		pps = append(pps, matches...)
	}
	return pps
}

// markMissingDisconnected 将已不存在的端口标记为断开，调用方须持有 m.mu。
// 返回是否有连接状态发生变化。
func (m *ModemService) markMissingDisconnected(devs []string) bool {
	available := make(map[string]struct{}, len(devs))
	for _, dev := range devs {
		available[path.Base(dev)] = struct{}{}
	}

	changed := false
	for name, conn := range m.pool {
		if _, ok := available[name]; ok {
			continue
		}
		if conn == nil {
			delete(m.pool, name)
			continue
		}
		if conn.setConnected(false) {
			log.Printf("[%s] disconnected: serial port disappeared", name)
			m.InvalidateModemCache(name)
			changed = true
		}
		if err := conn.Close(); err != nil {
			log.Printf("[%s] close disappeared serial port failed: %v", name, err)
		}
	}
	return changed
}

// StartModemWatch 启动后台设备监视：启动时立即扫描一次；之后每个 interval 检查，
// 存在断开端口（断线重连）或距上次扫描超过 modemRefreshInterval（发现新设备、
// 校验在线端口）时重新扫描，使设备列表快照保持新鲜。
func (m *ModemService) StartModemWatch(interval time.Duration) {
	go func() {
		m.RescanModems()
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for range ticker.C {
			if m.hasDisconnected() {
				log.Printf("modem watch: detected disconnected port(s), rescanning")
				m.ScanModems()
				continue
			}
			if time.Since(m.lastScanCompletedAt()) >= modemRefreshInterval {
				m.ScanModems()
			}
		}
	}()
}

// hasDisconnected 返回连接池中是否存在断开的端口。
func (m *ModemService) hasDisconnected() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, conn := range m.pool {
		if conn != nil && !conn.State().Connected {
			return true
		}
	}
	return false
}

// isPortCoolingDown 判断端口是否在连接失败冷却期内。
func (m *ModemService) isPortCoolingDown(u string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	t, ok := m.failedPorts[u]
	if !ok {
		return false
	}
	if time.Since(t) < portCooldown {
		return true
	}
	delete(m.failedPorts, u)
	return false
}

// markPortFailed 记录端口连接失败，启动冷却。
func (m *ModemService) markPortFailed(u string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.failedPorts[u] = time.Now()
}

// clearPortFailure 清除端口冷却记录（端口成功连接后调用）。
func (m *ModemService) clearPortFailure(u string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.failedPorts, u)
}

// scanThrottled 判断默认扫描是否被节流；未被节流则记录本次时间。
func (m *ModemService) scanThrottled() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	if time.Since(m.lastScan) < scanInterval {
		return true
	}
	m.lastScan = time.Now()
	return false
}

// notifyModemsChanged 通知前端设备列表发生变化；队列满时丢弃（前端仍可手动刷新）。
func (m *ModemService) notifyModemsChanged() {
	select {
	case ModemEvent <- ModemEventMsg{Event: ModemEventModems, Data: "changed"}:
	default:
	}
}

// GetConnList 返回连接池中的全部端口连接
func (m *ModemService) GetConnList() []*ModemConn {
	m.mu.Lock()
	defer m.mu.Unlock()

	conns := make([]*ModemConn, 0, len(m.pool))
	for _, model := range m.pool {
		conns = append(conns, model)
	}
	return conns
}

// ListModemStates 返回设备列表快照（按名称排序），connectedOnly 为 true 时只含在线设备。
// 只读内存状态，不访问串口。
func (m *ModemService) ListModemStates(connectedOnly bool) []ModemState {
	states := []ModemState{}
	for _, conn := range m.GetConnList() {
		if conn == nil {
			continue
		}
		state := conn.State()
		if connectedOnly && !state.Connected {
			continue
		}
		states = append(states, state)
	}
	sort.Slice(states, func(i, j int) bool { return states[i].Name < states[j].Name })
	return states
}

// GetConnectedNames 返回当前可用连接名称列表
func (m *ModemService) GetConnectedNames() []string {
	m.mu.Lock()
	defer m.mu.Unlock()

	names := []string{}
	for _, conn := range m.pool {
		if conn == nil || !conn.State().Connected || conn.Device == nil || !conn.IsOpen() {
			continue
		}
		names = append(names, conn.Name)
	}
	sort.Strings(names)
	return names
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
	if !conn.State().Connected || conn.Device == nil || !conn.IsOpen() {
		return nil, fmt.Errorf("[%s] not connected", n)
	}
	if !conn.SerialPortExists() {
		if conn.setConnected(false) {
			m.InvalidateModemCache(n)
			m.notifyModemsChanged()
		}
		if err := conn.Close(); err != nil {
			log.Printf("[%s] close missing serial port failed: %v", n, err)
		}
		return nil, fmt.Errorf("[%s] serial port disappeared", n)
	}
	return conn, nil
}

// LoadModemInfo 读取设备基本信息缓存；未命中或 force 时调用 load 现场查询并缓存。
func (m *ModemService) LoadModemInfo(name string, force bool, load func() (map[string]any, error)) (map[string]any, error) {
	return m.infoCache.GetOrLoad(path.Base(name), force, load)
}

// LoadModemSignal 读取信号缓存；未命中或 force 时调用 load 现场查询并缓存。
func (m *ModemService) LoadModemSignal(name string, force bool, load func() (map[string]any, error)) (map[string]any, error) {
	return m.signalCache.GetOrLoad(path.Base(name), force, load)
}

// InvalidateModemCache 使指定设备的信息/信号缓存失效。
// 任何可能改变 SIM、号码、注册状态的写操作完成后都应调用。
// 不使用 m.mu，持有 m.mu 时也可调用。
func (m *ModemService) InvalidateModemCache(name string) {
	n := path.Base(name)
	m.infoCache.Delete(n)
	m.signalCache.Delete(n)
}

// SetEuiccDetected 记录端口 SIM 是否为 eUICC，并按 IMEI 记忆，
// 端口重新枚举或重连后可直接恢复判定；状态变化时通知前端刷新设备列表。
func (m *ModemService) SetEuiccDetected(conn *ModemConn, isEuicc bool) {
	prev := conn.updateState(func(s *ModemState) { s.HasEuicc = isEuicc })
	if imei := prev.IMEI; imei != "" {
		m.mu.Lock()
		m.euiccIMEIs[imei] = isEuicc
		m.mu.Unlock()
	}
	if prev.HasEuicc != isEuicc {
		m.notifyModemsChanged()
	}
}

// knownEuicc 返回该 IMEI 之前的 eUICC 判定结果。
func (m *ModemService) knownEuicc(imei string) bool {
	if imei == "" {
		return false
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.euiccIMEIs[imei]
}

// handleIncomingSms 处理指定端口的新接收短信
func (m *ModemService) handleIncomingSms(portName string, smsIndex int) {
	conn, err := m.GetConn(portName)
	if err != nil {
		log.Printf("[%s] Failed to get connection for incoming Sms: %v", portName, err)
		return
	}

	// 与兜底轮询互斥，避免同一条短信被两条链路重复处理
	conn.smsMu.Lock()
	defer conn.smsMu.Unlock()

	// 获取短信列表（只获取新短信）
	smsList, err := conn.ListSmsPdu(4)
	if err != nil {
		log.Printf("[%s] Failed to list Sms: %v", portName, err)
		return
	}

	smsdbService := NewSmsdbService()
	webhookService := NewWebhookService()

	// 处理每条短信
	for _, atSms := range smsList {
		hasNewSms := false
		for _, idx := range atSms.Indices {
			if idx == smsIndex {
				hasNewSms = true
				break
			}
		}
		if hasNewSms {
			log.Printf("[%s] New Sms from %s: %s", portName, atSms.Number, atSms.Text)
			modelSms := atSmsToModelSms(atSms, conn)
			smsdbService.HandleIncomingSms(modelSms)
			webhookService.HandleIncomingSms(modelSms)
			// 同步删除设备上的短信；删除必须在 smsMu 内完成，
			// 否则兜底轮询可能在删除前列出同一条短信导致重复触发
			if err := conn.DeleteSms(atSms.Indices); err != nil {
				log.Printf("[%s] failed to delete Sms: %v", portName, err)
			} else {
				log.Printf("[%s] Sms deleted automatically, indices: %v", portName, atSms.Indices)
			}
		}
	}
}

// setupSmsNotification 显式配置新短信通知（AT+CNMI），确保短信存入存储后模组
// 主动上报 +CMTI URC。不能依赖模组出厂默认值：部分模组默认不上报，或在协议栈
// 复位/SIM 重注册后恢复为不上报，表现为入网状态正常但收信静默。
// mode=2 链路忙时缓存 URC 稍后补发，个别模组不支持时降级为 mode=1。
func setupSmsNotification(modem *at.Device, pf func(string, ...any)) {
	for _, cmd := range []string{"AT+CNMI=2,1,0,0,0", "AT+CNMI=1,1,0,0,0"} {
		if err := modem.SendExpect(cmd, "OK"); err == nil {
			pf("sms notification configured: %s", cmd)
			return
		}
	}
	pf("failed to configure sms notification (AT+CNMI), incoming sms may not be reported")
}

// makeConnect 添加新的 AT 接口；端口已在线且可响应时直接返回。
func (m *ModemService) makeConnect(u string) (err error) {
	n := path.Base(u)

	// 创建日志函数
	pf := func(s string, v ...any) {
		log.Printf(fmt.Sprintf("[%s] %s", n, s), v...)
	}

	// 冷却期内跳过近期失败的端口（多为 diag/数据等非 AT 口），避免反复重试拖慢扫描
	if m.isPortCoolingDown(u) {
		return fmt.Errorf("[%s] skipped, recently failed", n)
	}

	// 检查是否已连接：只在锁内取出连接，AT 测试在锁外进行。
	// 端口正被长操作（eSIM 会话、短信轮询等）占用时 Test 会等待 opMu，
	// 若在 m.mu 内等待会卡住所有 GetConn，导致整个 Web 界面无响应。
	m.mu.Lock()
	existing := m.pool[n]
	m.mu.Unlock()
	if existing != nil {
		if existing.Test() == nil {
			if existing.setConnected(true) {
				m.notifyModemsChanged()
			}
			return nil
		}
		if existing.setConnected(false) {
			pf("disconnected: at test failed")
			m.InvalidateModemCache(n)
			m.notifyModemsChanged()
		}
		existing.Close()
	}

	// 连接失败时记录冷却，下次扫描短期内跳过该端口
	defer func() {
		if err != nil {
			m.markPortFailed(u)
		}
	}()

	// 创建事件处理函数
	hf := func(e string, p map[int]string) {
		select {
		case ModemEvent <- ModemEventMsg{Event: ModemEventURC, Data: fmt.Sprintf("%s, %s, %v", n, e, p)}:
		default:
			log.Printf("[%s] urc dropped: %s", n, e)
		}
		m.dispatchURC(n, e, p)
		// 处理收到的短信通知
		if e == "+CMTI" && len(p) > 0 {
			if indexStr, ok := p[1]; ok {
				if index, err := strconv.Atoi(indexStr); err == nil {
					m.handleIncomingSms(n, index)
				}
			}
		}
	}

	// 以下为耗时操作，不持有 m.mu 锁

	// 打开串口
	pf("connecting")
	port, err := serial.Open(u, &serial.Mode{
		BaudRate: 115200,
		DataBits: 8,
		Parity:   serial.NoParity,
		StopBits: serial.OneStopBit,
	})
	if err != nil {
		pf("connect failed: %v", err)
		return err
	}
	if err := port.SetReadTimeout(1 * time.Second); err != nil {
		port.Close()
		pf("set read timeout failed: %v", err)
		return err
	}

	// 链接新设备；注册厂商 ping URC 前缀，这些结果在 OK 之后异步上报，
	// 不注册会被当作残留响应在下一条命令前被清空
	notifySet := at.DefaultNotificationSet()
	notifySet.CustomNotify = pingURCPrefixes
	modem := at.New(&serialPort{Port: port}, hf, &at.Config{Printf: pf, Timeout: 2 * time.Second, NotificationSet: notifySet})
	if err := modem.Test(); err != nil {
		pf("at test failed: %v", err)
		modem.Close()
		return err
	}

	// 设置默认参数
	modem.EchoOff()     // 关闭回显
	modem.SetSmsMode(0) // PDU 模式
	modem.SetSmsStore("ME", "ME", "ME")
	setupSmsNotification(modem, pf)

	state := ModemState{Name: n, Number: "unknown", Connected: true}

	if imei, err := modem.GetIMEI(); err == nil {
		state.IMEI = strings.TrimSpace(imei)
	} else {
		pf("connected, but failed to get IMEI: %v", err)
	}

	if number, _, err := modem.GetNumber(); err == nil {
		pf("connected, phone number: %s", number)
		state.Number = number
	} else {
		pf("connected, but failed to get phone number: %v", err)
	}

	// 厂商/型号/固件在连接生命周期内不变，连接时读取一次，信息查询直接复用
	if v, err := modem.GetManufacturer(); err == nil {
		state.Manufacturer = strings.TrimSpace(v)
	}
	if v, err := modem.GetModel(); err == nil {
		state.Model = strings.TrimSpace(v)
	}
	if v, err := modem.GetRevision(); err == nil {
		state.Revision = strings.TrimSpace(v)
	}

	// 注意：此处不主动探测 eUICC。ML307A 对 AT+CSIM 的 MANAGE CHANNEL /
	// SELECT 等 APDU 透传敏感，连接后立即探测会触发 USB 重新枚举，并与
	// 断线重连形成「探测→掉线→重连→再探测」循环。HasEuicc 由读取 eSIM 时
	// 惰性判定，并按 IMEI 记忆，重连后在此恢复。
	state.HasEuicc = m.knownEuicc(state.IMEI)

	conn := &ModemConn{
		Name:   n,
		Path:   u,
		state:  state,
		Device: modem,
	}

	// 短暂持锁：写入连接池
	m.mu.Lock()
	m.pool[n] = conn
	m.mu.Unlock()

	// 连接成功，清除该端口的失败冷却记录和旧连接的信息缓存
	m.clearPortFailure(u)
	m.InvalidateModemCache(n)
	m.notifyModemsChanged()

	return nil
}
