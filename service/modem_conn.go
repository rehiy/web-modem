package service

import (
	"errors"
	"fmt"
	"log"
	"os"
	"runtime"
	"strings"
	"time"

	"github.com/rehiy/modem/at"
)

// ErrCommandUnsupported 表示模块不支持该可选 AT 指令（曾返回裸 ERROR）。
var ErrCommandUnsupported = errors.New("command not supported by modem")

// State 返回连接状态的副本，可安全用于序列化输出。
func (c *ModemConn) State() ModemState {
	c.stateMu.RLock()
	defer c.stateMu.RUnlock()
	return c.state
}

// updateState 在 stateMu 保护下修改状态，返回修改前的副本。
func (c *ModemConn) updateState(fn func(*ModemState)) ModemState {
	c.stateMu.Lock()
	defer c.stateMu.Unlock()
	prev := c.state
	fn(&c.state)
	return prev
}

// SetNumber 更新缓存的本机号码。
func (c *ModemConn) SetNumber(v string) {
	c.updateState(func(s *ModemState) { s.Number = strings.TrimSpace(v) })
}

// SetIMEI 更新缓存的 IMEI。
func (c *ModemConn) SetIMEI(v string) {
	c.updateState(func(s *ModemState) { s.IMEI = strings.TrimSpace(v) })
}

// SetICCID 更新缓存的 ICCID。
func (c *ModemConn) SetICCID(v string) {
	c.updateState(func(s *ModemState) { s.ICCID = strings.TrimSpace(v) })
}

// SetIMSI 更新缓存的 IMSI。
func (c *ModemConn) SetIMSI(v string) {
	c.updateState(func(s *ModemState) { s.IMSI = strings.TrimSpace(v) })
}

// SetOperator 更新缓存的运营商。
func (c *ModemConn) SetOperator(v string) {
	c.updateState(func(s *ModemState) { s.Operator = strings.TrimSpace(v) })
}

// setConnected 更新连接标记，返回值表示状态是否发生变化。
func (c *ModemConn) setConnected(v bool) bool {
	return c.updateState(func(s *ModemState) { s.Connected = v }).Connected != v
}

// SendOptionalCommand 发送可选的（厂商扩展/非所有模块都支持的）查询指令。
// 模块对某条指令返回裸 ERROR 时记为不支持，本连接生命周期内直接跳过，
// 避免每次刷新信息都在串口上浪费一次往返；重连后会重新探测。
func (c *ModemConn) SendOptionalCommand(cmd string) ([]string, error) {
	c.stateMu.RLock()
	_, skip := c.unsupported[cmd]
	c.stateMu.RUnlock()
	if skip {
		return nil, ErrCommandUnsupported
	}

	resp, err := c.SendCommand(cmd)
	if err != nil || !hasBareError(resp) {
		return resp, err
	}
	c.stateMu.Lock()
	if c.unsupported == nil {
		c.unsupported = map[string]struct{}{}
	}
	c.unsupported[cmd] = struct{}{}
	c.stateMu.Unlock()
	return resp, ErrCommandUnsupported
}

// hasBareError 判断响应是否以裸 ERROR 结束（区别于带原因码的 CME/CMS ERROR）。
func hasBareError(resp []string) bool {
	for _, line := range resp {
		if strings.EqualFold(strings.TrimSpace(line), "ERROR") {
			return true
		}
	}
	return false
}

func (c *ModemConn) SerialPortExists() bool {
	if runtime.GOOS == "windows" || c.Path == "" {
		return true
	}
	_, err := os.Stat(c.Path)
	return err == nil
}

// IsOpen 与 Close 不获取 opMu：二者在持有全局 m.mu 时被调用，若端口正被长操作
// （eSIM 会话/下载等）占用，等待 opMu 会连带卡住所有设备的请求。
// at.Device 的 IsOpen 为原子读、Close 幂等，无需与 AT 指令串行；
// 关闭时进行中的指令会以 "device closed" 结束。
func (c *ModemConn) IsOpen() bool {
	return c.Device != nil && c.Device.IsOpen()
}

func (c *ModemConn) Close() error {
	if c.Device == nil {
		return nil
	}
	return c.Device.Close()
}

func (c *ModemConn) Test() error {
	c.opMu.Lock()
	defer c.opMu.Unlock()
	return c.Device.Test()
}

func (c *ModemConn) EchoOff() error {
	c.opMu.Lock()
	defer c.opMu.Unlock()
	return c.Device.EchoOff()
}

func (c *ModemConn) SetSmsMode(v int) error {
	c.opMu.Lock()
	defer c.opMu.Unlock()
	return c.Device.SetSmsMode(v)
}

func (c *ModemConn) SetSmsStore(v1, v2, v3 string) error {
	c.opMu.Lock()
	defer c.opMu.Unlock()
	return c.Device.SetSmsStore(v1, v2, v3)
}

func (c *ModemConn) SendCommand(cmd string) ([]string, error) {
	c.opMu.Lock()
	defer c.opMu.Unlock()
	return c.Device.SendCommand(cmd)
}

func (c *ModemConn) SendCommandWithTimeout(cmd string, timeout time.Duration) ([]string, error) {
	c.opMu.Lock()
	defer c.opMu.Unlock()
	return c.Device.SendCommandWithTimeout(cmd, timeout)
}

func (c *ModemConn) SendSmsPdu(number, message string) error {
	c.opMu.Lock()
	defer c.opMu.Unlock()
	return c.Device.SendSmsPdu(number, message)
}

func (c *ModemConn) ListSmsPdu(stat int) ([]at.Sms, error) {
	c.opMu.Lock()
	defer c.opMu.Unlock()
	list, err := c.Device.ListSmsPdu(stat)
	// SMS_DEBUG=1 时记录每条短信的原始 PDU，用于定位短信解码 bug（库内部为黑盒）
	if err == nil && os.Getenv("SMS_DEBUG") != "" {
		for _, s := range list {
			if len(s.Indices) > 0 {
				c.logSmsPduLocked(s.Indices[0])
			}
		}
	}
	return list, err
}

// logSmsPduLocked 用 AT+CMGR 重新读取指定索引的原始 PDU 并打印日志。
// 调用方必须已持有 opMu；用 c.Device 直发以避免重复加锁。
func (c *ModemConn) logSmsPduLocked(index int) {
	resp, err := c.Device.SendCommandWithTimeout(fmt.Sprintf("AT+CMGR=%d", index), 5*time.Second)
	if err != nil {
		return
	}
	for _, line := range resp {
		line = strings.TrimSpace(line)
		if len(line) >= 4 && isHexPdu(line) {
			log.Printf("[SMS PDU] index=%d pdu=%s", index, line)
			return
		}
	}
}

// isHexPdu 判断字符串是否为纯十六进制（PDU 行特征）。
func isHexPdu(s string) bool {
	for _, r := range s {
		if !((r >= '0' && r <= '9') || (r >= 'A' && r <= 'F') || (r >= 'a' && r <= 'f')) {
			return false
		}
	}
	return true
}

func (c *ModemConn) DeleteSms(indices []int) error {
	c.opMu.Lock()
	defer c.opMu.Unlock()
	return c.Device.DeleteSms(indices)
}

func (c *ModemConn) GetManufacturer() (string, error) {
	c.opMu.Lock()
	defer c.opMu.Unlock()
	return c.Device.GetManufacturer()
}

func (c *ModemConn) GetModel() (string, error) {
	c.opMu.Lock()
	defer c.opMu.Unlock()
	return c.Device.GetModel()
}

func (c *ModemConn) GetRevision() (string, error) {
	c.opMu.Lock()
	defer c.opMu.Unlock()
	return c.Device.GetRevision()
}

func (c *ModemConn) GetIMEI() (string, error) {
	c.opMu.Lock()
	defer c.opMu.Unlock()
	return c.Device.GetIMEI()
}

func (c *ModemConn) GetICCID() (string, error) {
	c.opMu.Lock()
	defer c.opMu.Unlock()
	return c.Device.GetICCID()
}

func (c *ModemConn) GetIMSI() (string, error) {
	c.opMu.Lock()
	defer c.opMu.Unlock()
	return c.Device.GetIMSI()
}

func (c *ModemConn) GetSIMStatus() (string, error) {
	c.opMu.Lock()
	defer c.opMu.Unlock()
	return c.Device.GetSIMStatus()
}

func (c *ModemConn) VerifyPIN(pin string) error {
	c.opMu.Lock()
	defer c.opMu.Unlock()
	return c.Device.VerifyPIN(pin)
}

func (c *ModemConn) ChangePIN(oldPIN, newPIN string) error {
	c.opMu.Lock()
	defer c.opMu.Unlock()
	return c.Device.ChangePIN(oldPIN, newPIN)
}

func (c *ModemConn) UnlockPIN(pinType string, enable bool, password string) error {
	c.opMu.Lock()
	defer c.opMu.Unlock()
	return c.Device.UnlockPIN(pinType, enable, password)
}

func (c *ModemConn) GetNumber() (string, int, error) {
	c.opMu.Lock()
	defer c.opMu.Unlock()
	return c.Device.GetNumber()
}

func (c *ModemConn) GetOperator() (int, int, string, int, error) {
	c.opMu.Lock()
	defer c.opMu.Unlock()
	return c.Device.GetOperator()
}

func (c *ModemConn) GetNetworkMode() (int, error) {
	c.opMu.Lock()
	defer c.opMu.Unlock()
	return c.Device.GetNetworkMode()
}

func (c *ModemConn) GetNetworkStatus() (int, int, error) {
	c.opMu.Lock()
	defer c.opMu.Unlock()
	return c.Device.GetNetworkStatus()
}

func (c *ModemConn) GetGPRSStatus() (int, int, error) {
	c.opMu.Lock()
	defer c.opMu.Unlock()
	return c.Device.GetGPRSStatus()
}

func (c *ModemConn) GetSignalQuality() (int, int, error) {
	c.opMu.Lock()
	defer c.opMu.Unlock()
	return c.Device.GetSignalQuality()
}

func (c *ModemConn) SetAPN(cid int, pdpType, apn string) error {
	c.opMu.Lock()
	defer c.opMu.Unlock()
	return c.Device.SetAPN(cid, pdpType, apn)
}

func (c *ModemConn) SetPDPContext(cid int, state int) error {
	c.opMu.Lock()
	defer c.opMu.Unlock()
	return c.Device.SetPDPContext(cid, state)
}

func (c *ModemConn) GetSmsCenter() (string, int, error) {
	c.opMu.Lock()
	defer c.opMu.Unlock()
	return c.Device.GetSmsCenter()
}

func (c *ModemConn) GetSmsMode() (int, error) {
	c.opMu.Lock()
	defer c.opMu.Unlock()
	return c.Device.GetSmsMode()
}

func (c *ModemConn) GetSmsStore() (map[string]any, error) {
	c.opMu.Lock()
	defer c.opMu.Unlock()
	return c.Device.GetSmsStore()
}

// SendCSIM 通过 AT+CSIM 在基础通道透传一条 APDU，返回十六进制响应（含 SW1SW2）。
// 部分模块固件禁用了 AT+CCHO/CGLA 逻辑通道命令，CSIM 更通用，逻辑通道改由
// MANAGE CHANNEL APDU 自行管理。
func (c *ModemConn) SendCSIM(apduHex string) (string, error) {
	c.opMu.Lock()
	defer c.opMu.Unlock()
	return c.sendCSIMLocked(apduHex)
}

// sendCSIMLocked 是 SendCSIM 的无锁版本，调用方必须已持有 opMu。
// 供 eUICC 会话整体持锁使用，避免逻辑通道打开期间被其他 AT 命令插入，
// 也防止多段 APDU 之间被基础 AT 命令打断导致模块 USB 重新枚举。
func (c *ModemConn) sendCSIMLocked(apduHex string) (string, error) {
	cmd := fmt.Sprintf(`AT+CSIM=%d,"%s"`, len(apduHex), apduHex)
	resp, err := c.Device.SendCommandWithTimeout(cmd, 10*time.Second)
	if err != nil {
		return "", err
	}
	for _, line := range resp {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "+CSIM:") {
			// +CSIM: <length>,"<hex>"
			_, hexPart, ok := strings.Cut(line, `"`)
			if !ok {
				return "", fmt.Errorf("CSIM: malformed response: %s", line)
			}
			hexPart = strings.TrimSuffix(hexPart, `"`)
			return strings.ToUpper(strings.TrimSpace(hexPart)), nil
		}
	}
	return "", fmt.Errorf("CSIM: no response: %v", resp)
}

// LockOp 获取连接的操作锁，供需要整体串行执行的会话使用。
// 调用方负责 defer Unlock()。返回的解锁函数便于 defer 调用。
func (c *ModemConn) LockOp() func() {
	c.opMu.Lock()
	return c.opMu.Unlock
}
