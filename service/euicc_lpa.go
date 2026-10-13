package service

import (
	"context"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/damonto/euicc-go/bertlv"
	"github.com/damonto/euicc-go/lpa"
	sgp22 "github.com/damonto/euicc-go/v2"

	"github.com/rehiy/web-modem/database"
)

// csimChannel 将 ModemConn 的 AT+CSIM 透传通道适配为 euicc-go 的 apdu.SmartCardChannel。
//
// 设计要点：
//  1. 不自行打开串口——串口由 ModemService 统一管理，Connect/Disconnect 为空操作，
//     仅在 Connect 中校验连接可用性。
//  2. OpenLogicalChannel 用 MANAGE CHANNEL + SELECT 实现（与现有 openIsdrChannel 一致），
//     不使用 AT+CCHO（部分模块固件，如 ML307A，会以 CME ERROR:3 拒绝该命令）。
//  3. Transmit 为“裸发”：仅做 []byte↔hex 透传，原样返回完整响应（数据 + SW1SW2）。
//     euicc-go 上层（apdu 传输层）自行处理 CLA 通道编入、STORE DATA 分块、
//     61xx GET RESPONSE 续传，本方法不得叠加这些逻辑以免重复处理。
//  4. 锁由调用方通过 withLpa 在整个 lpa.Client 生命周期内持有，
//     以满足 sendCSIMLocked “调用方已持 opMu” 的约束。
type csimChannel struct {
	conn *ModemConn
}

func newCsimChannel(conn *ModemConn) *csimChannel {
	return &csimChannel{conn: conn}
}

// Connect 校验连接可用。euicc-go 原生 AT 驱动在此发送 80AA 终端能力上报
// （TERMINAL CAPABILITY），本项目所用模块对该命令支持不一且非必需，故跳过。
func (c *csimChannel) Connect() error {
	if c.conn == nil {
		return fmt.Errorf("modem connection is nil")
	}
	return nil
}

// Disconnect 为空操作：串口生命周期由 ModemService 管理，此处不关闭。
func (c *csimChannel) Disconnect() error {
	return nil
}

// OpenLogicalChannel 打开逻辑通道并在其上 SELECT 指定 AID（通常为 ISD-R）。
func (c *csimChannel) OpenLogicalChannel(AID []byte) (byte, error) {
	resp, err := c.conn.sendCSIMLocked("0070000001")
	if err != nil {
		return 0, fmt.Errorf("MANAGE CHANNEL open: %w", err)
	}
	data, sw, err := splitAPDUResponse(resp)
	if err != nil {
		return 0, err
	}
	if sw != "9000" || len(data) < 1 {
		return 0, fmt.Errorf("MANAGE CHANNEL open failed SW=%s", sw)
	}
	ch := data[0]

	// SELECT AID：CLA 编入通道号，A4 04 00 Lc <AID> Le
	selectHex := fmt.Sprintf("%02XA40400%02X%s00", ch, len(AID), strings.ToUpper(hex.EncodeToString(AID)))
	resp, err = c.conn.sendCSIMLocked(selectHex)
	if err != nil {
		_ = c.closeChannel(ch)
		return 0, fmt.Errorf("SELECT AID: %w", err)
	}
	_, sw, err = splitAPDUResponse(resp)
	if err != nil {
		_ = c.closeChannel(ch)
		return 0, err
	}
	// SELECT 成功返回 9000 或 61xx（有待读数据），其余视为失败
	if sw != "9000" && !strings.HasPrefix(sw, "61") {
		_ = c.closeChannel(ch)
		return 0, fmt.Errorf("SELECT AID failed SW=%s", sw)
	}
	return ch, nil
}

// Transmit 裸发一条 APDU，返回完整响应字节（数据 + SW1SW2）。
// 返回值必须包含 SW：euicc-go 的 apdu.Response 依赖末尾两字节判定 OK/HasMore。
func (c *csimChannel) Transmit(command []byte) ([]byte, error) {
	apduHex := strings.ToUpper(hex.EncodeToString(command))
	resp, err := c.conn.sendCSIMLocked(apduHex)
	if err != nil {
		return nil, err
	}
	full, err := hex.DecodeString(resp)
	if err != nil {
		return nil, fmt.Errorf("decode CSIM response: %w", err)
	}
	if len(full) < 2 {
		return nil, fmt.Errorf("CSIM response too short: %X", full)
	}
	return full, nil
}

// CloseLogicalChannel 关闭指定逻辑通道（MANAGE CHANNEL CLOSE: 00 70 80 <ch> 00）。
func (c *csimChannel) CloseLogicalChannel(channel byte) error {
	return c.closeChannel(channel)
}

func (c *csimChannel) closeChannel(channel byte) error {
	_, err := c.conn.sendCSIMLocked(fmt.Sprintf("007080%02X00", channel))
	return err
}

// withLpa 在一次 opMu 持锁内创建并运行 lpa.Client。
//
// 整个 Client 生命周期（含 ISD-R 通道的打开与关闭）都在锁内完成，
// 既满足 sendCSIMLocked “调用方已持锁” 的约束，也避免 APDU 会话被
// 基础 AT 命令插入导致模块不稳定或 USB 重新枚举。
//
// 注意：fn 内若发起长时间 HTTP（如下载 Profile），持锁会阻塞该连接的其它 AT 操作。
// 读取类操作持锁时间短，可放心使用；下载类操作需另行评估锁粒度。
func withLpa(conn *ModemConn, fn func(*lpa.Client) error) error {
	unlock := conn.LockOp()
	defer unlock()

	client, err := lpa.New(&lpa.Options{Channel: newCsimChannel(conn)})
	if err != nil {
		return err
	}
	defer client.Close()
	return fn(client)
}

// GetEuiccInfoViaLpa 基于 euicc-go（完整 LPAd 实现）读取 EID 与 Profile 列表。
//
// 与 Profile 写操作和下载共用 csimChannel，返回前补充按 ICCID 登记的号码。
func GetEuiccInfoViaLpa(conn *ModemConn) (EuiccInfo, error) {
	var info EuiccInfo
	err := withLpa(conn, func(client *lpa.Client) error {
		eid, err := client.EID()
		if err != nil {
			return fmt.Errorf("GET EID: %w", err)
		}
		info.EID = strings.ToUpper(hex.EncodeToString(eid))

		profiles, err := client.ListProfile(nil, nil)
		if err != nil {
			return fmt.Errorf("LIST PROFILES: %w", err)
		}
		info.Profiles = make([]EuiccProfile, 0, len(profiles))
		for _, p := range profiles {
			ep := EuiccProfile{
				ICCID:               p.ICCID.String(),
				Nickname:            p.ProfileNickname,
				ServiceProviderName: p.ServiceProviderName,
				ProfileName:         p.ProfileName,
			}
			if p.ProfileState != sgp22.ProfileDisabled {
				ep.State = "enabled"
				ep.IsEnabled = true
			} else {
				ep.State = "disabled"
			}
			info.Profiles = append(info.Profiles, ep)
		}
		return nil
	})
	if err != nil {
		if isNotEuiccError(err) {
			GetModemService().SetEuiccDetected(conn, false)
		}
		return info, err
	}
	GetModemService().SetEuiccDetected(conn, true)
	enrichProfileNumbers(conn, &info)
	return info, nil
}

// isNotEuiccError 判断错误是否表明该 SIM 不是 eUICC（ISD-R AID 不存在）。
func isNotEuiccError(err error) bool {
	msg := err.Error()
	return strings.Contains(msg, "SW=6A82") || strings.Contains(msg, "SW=6B00")
}

// enrichProfileNumbers 为 Profile 列表补充手机号。
//
// 模块只能读取当前启用 Profile 的号码，且切卡复位期间读到的号码可能属于旧卡。
// 因此 Profile 列表只展示按 ICCID 保存在 SQLite 中、由用户手动确认的号码，
// 绝不使用模块瞬时返回值覆盖登记记录。未登记的 Profile 显示为空。
func enrichProfileNumbers(conn *ModemConn, info *EuiccInfo) {
	if info == nil || len(info.Profiles) == 0 {
		return
	}
	iccids := make([]string, 0, len(info.Profiles))
	for _, p := range info.Profiles {
		iccids = append(iccids, p.ICCID)
	}
	cached := database.GetProfileNumbers(iccids)
	for i := range info.Profiles {
		p := &info.Profiles[i]
		if p.IsEnabled {
			// 仅同步当前 ICCID 缓存；号码始终来自已确认的 SQLite 记录。
			conn.SetICCID(p.ICCID)
		}
		if n := cached[p.ICCID]; n != "" {
			p.Number = n
		}
	}
}

// EuiccInfo2Data 是 EUICCInfo2 的解析结果。Raw 保留完整 hex，其余为尽力提取的可读字段。
type EuiccInfo2Data struct {
	Raw             string `json:"raw"`
	ProfileVersion  string `json:"profileVersion,omitempty"`  // tag 81，如 "2.5.0"
	FirmwareVersion string `json:"firmwareVersion,omitempty"` // tag 83
	ExtCardResource string `json:"extCardResource,omitempty"` // tag 84，hex
}

// EuiccConfiguredAddresses 是 eUICC 配置的默认服务器地址（SM-DP+/SM-DS）。
type EuiccConfiguredAddresses struct {
	DefaultSMDPAddress string `json:"defaultSmdpAddress"`
	RootSMDSAddress    string `json:"rootSmdsAddress"`
}

// EuiccNotification 是一条本地待处理通知的元数据。
type EuiccNotification struct {
	SequenceNumber    int64  `json:"sequenceNumber"`
	NotificationEvent string `json:"notificationEvent"` // install/enable/disable/delete
	Address           string `json:"address"`
	ICCID             string `json:"iccid"`
}

// GetEuiccInfo2ViaLpa 读取 EUICCInfo2（BF20），返回完整 hex 及尽力解析的版本字段。
func GetEuiccInfo2ViaLpa(conn *ModemConn) (EuiccInfo2Data, error) {
	var data EuiccInfo2Data
	err := withLpa(conn, func(client *lpa.Client) error {
		tlv, err := client.EUICCInfo2()
		if err != nil {
			return fmt.Errorf("GET EUICCInfo2: %w", err)
		}
		data.Raw = strings.ToUpper(hex.EncodeToString(tlv.Bytes()))
		if v := tlv.First(bertlv.Tag{0x81}); v != nil {
			data.ProfileVersion = versionString(v.Value)
		}
		if v := tlv.First(bertlv.Tag{0x83}); v != nil {
			data.FirmwareVersion = versionString(v.Value)
		}
		if v := tlv.First(bertlv.Tag{0x84}); v != nil {
			data.ExtCardResource = strings.ToUpper(hex.EncodeToString(v.Value))
		}
		return nil
	})
	return data, err
}

// GetEuiccConfiguredAddressesViaLpa 读取默认 SM-DP+ 与 SM-DS 地址。
func GetEuiccConfiguredAddressesViaLpa(conn *ModemConn) (EuiccConfiguredAddresses, error) {
	var addr EuiccConfiguredAddresses
	err := withLpa(conn, func(client *lpa.Client) error {
		a, err := client.EUICCConfiguredAddresses()
		if err != nil {
			return fmt.Errorf("GET ConfiguredAddresses: %w", err)
		}
		addr.DefaultSMDPAddress = a.DefaultSMDPAddress
		addr.RootSMDSAddress = a.RootSMDSAddress
		return nil
	})
	return addr, err
}

// SetDefaultSMDPAddressViaLpa 设置默认 SM-DP+ 地址（下载 Profile 的前置配置）。
func SetDefaultSMDPAddressViaLpa(conn *ModemConn, address string) error {
	return withLpa(conn, func(client *lpa.Client) error {
		if err := client.SetDefaultDPAddress(address); err != nil {
			return fmt.Errorf("SET DefaultDPAddress: %w", err)
		}
		return nil
	})
}

// ListEuiccNotificationsViaLpa 列出本地待处理通知的元数据。
func ListEuiccNotificationsViaLpa(conn *ModemConn) ([]EuiccNotification, error) {
	var list []EuiccNotification
	err := withLpa(conn, func(client *lpa.Client) error {
		metas, err := client.ListNotification()
		if err != nil {
			return fmt.Errorf("LIST Notification: %w", err)
		}
		list = make([]EuiccNotification, 0, len(metas))
		for _, m := range metas {
			list = append(list, EuiccNotification{
				SequenceNumber:    int64(m.SequenceNumber),
				NotificationEvent: notificationEventName(byte(m.ProfileManagementOperation)),
				Address:           m.Address,
				ICCID:             m.ICCID.String(),
			})
		}
		return nil
	})
	return list, err
}

// versionString 把 SGP.22 VersionType（3 字节 BCD，如 02 05 00）转为 "x.y.z"。
func versionString(b []byte) string {
	if len(b) < 3 {
		return ""
	}
	return fmt.Sprintf("%d.%d.%d", b[0], b[1], b[2])
}

// notificationEventName 把通知事件字节映射为可读名称（基于 SGP.22 NotificationEvent）。
func notificationEventName(b byte) string {
	switch b {
	case 0:
		return "install"
	case 1:
		return "enable"
	case 2:
		return "disable"
	case 3:
		return "delete"
	default:
		return fmt.Sprintf("unknown(%d)", b)
	}
}

// HandleEuiccNotificationViaLpa 上报指定序号的待处理通知回执给 SM-DP+/SM-DS，
// 并在上报成功后从本地列表删除。
func HandleEuiccNotificationViaLpa(conn *ModemConn, seq int64) error {
	return withLpa(conn, func(client *lpa.Client) error {
		pending, err := client.RetrieveNotificationList(sgp22.SequenceNumber(seq))
		if err != nil {
			return fmt.Errorf("检索通知: %w", err)
		}
		if len(pending) == 0 {
			return fmt.Errorf("未找到序号 %d 的通知", seq)
		}
		if err := client.HandleNotification(pending[0]); err != nil {
			return fmt.Errorf("上报回执: %w", err)
		}
		if err := client.RemoveNotificationFromList(sgp22.SequenceNumber(seq)); err != nil {
			return fmt.Errorf("删除已上报通知: %w", err)
		}
		return nil
	})
}

// DownloadResult 是下载 Profile 的结果。
type DownloadResult struct {
	ISDPAID string   `json:"isdpaid,omitempty"` // 新 Profile 的 ISD-P AID（hex）
	Stages  []string `json:"stages"`            // 已经过的下载阶段
}

// DownloadProfileViaLpa 用激活码（LPA:1$smdp$matchingId）下载并安装 Profile。
//
// 流程：解析激活码 → 以设备 IMEI 发起 ES9/ES8 全流程（认证 + 取 BPP + 安装）。
// IMEI 取自 ModemConn 缓存（连接时读取）；OnConfirm 自动通过（信任激活码来源）；
// 若激活码含确认码则自动带入。下载含多次 HTTPS 与多段 APDU，耗时较长，
// 整个过程在 opMu 持锁内完成（会阻塞该连接的其它 AT 操作）。
func DownloadProfileViaLpa(conn *ModemConn, activationCode, imei string) (DownloadResult, error) {
	var result DownloadResult
	err := withLpa(conn, func(client *lpa.Client) error {
		ac := &lpa.ActivationCode{}
		if err := ac.UnmarshalText([]byte(activationCode)); err != nil {
			return fmt.Errorf("解析激活码: %w", err)
		}
		// 优先使用手动输入的 IMEI；否则用设备 IMEI。部分模块（如 ML307A）
		// AT+CGSN 返回的 IMEI 含非数字字符或长度异常，需手动提供有效 15 位 IMEI。
		rawIMEI := strings.TrimSpace(imei)
		if rawIMEI == "" {
			rawIMEI = conn.State().IMEI
		}
		cleaned := stripNonDigits(rawIMEI)
		if len(cleaned) != 15 {
			return fmt.Errorf("IMEI 无效（%q → %q），下载需要 15 位数字 IMEI，请手动输入", rawIMEI, cleaned)
		}
		ac.IMEI = cleaned

		opts := &lpa.DownloadOptions{
			OnProgress: func(stage lpa.DownloadStage) {
				result.Stages = append(result.Stages, stage.String())
			},
			OnConfirm: func(*sgp22.ProfileInfo) bool {
				return true // 自动确认：用户已通过扫码/输入提供激活码，信任来源
			},
			OnEnterConfirmationCode: func() string {
				return ac.ConfirmationCode
			},
		}

		resp, err := client.DownloadProfile(context.Background(), ac, opts)
		if err != nil {
			return fmt.Errorf("下载 Profile: %w", err)
		}
		if resp != nil {
			result.ISDPAID = strings.ToUpper(hex.EncodeToString(resp.ISDPAID()))
		}
		return nil
	})
	return result, err
}

// EnableProfileViaLpa 启用（切换到）指定 ICCID 的 Profile（refresh=true，切换后刷新）。
//
// 启用会触发 SIM REFRESH 复位，可能打断末条 APDU 响应（表现为 SW=910B、
// CSIM 被拒等传输类错误），使库层报错但实际切换已完成。
// 因此对这类疑似 REFRESH 打断的错误做事后核验：等待模块复位完成后，
// 用基础 AT 命令读取当前活动 ICCID 与目标比对。若模块已因 REFRESH 拒绝
// 后续 CSIM 命令，则将该传输中断视为“切换已受理”，由调用方在复位后刷新列表。
func EnableProfileViaLpa(conn *ModemConn, iccid string) error {
	// 切卡会改变 ICCID/IMSI/号码/注册状态，结束后使设备信息缓存失效
	defer GetModemService().InvalidateModemCache(conn.Name)
	err := profileCommandViaLpa(conn, iccid, true)
	if err == nil {
		return nil
	}
	// 91xx 是 SIM Toolkit 的正常响应：Profile 切换已被 eUICC 接受，
	// 后续的 REFRESH Proactive Command 会重置模块并打断 APDU 会话。
	// 此时不能把库层的“非 9000”包装错误回传给用户，否则会出现
	// 切换成功但 API 返回 500 的假失败。
	if isAcceptedProfileSwitchStatus(err) {
		return nil
	}
	if !isLikelyRefreshDisruption(err) {
		return err
	}
	if activeICCIDMatches(conn, iccid, 12*time.Second) {
		return nil
	}
	// 部分模块会在处理 REFRESH 时提前断开 CSIM 会话。此时本次 EnableProfile
	// 调用已经发出，且无法再通过 AT+CSIM 或 ICCID 读取完成同步确认；把它
	// 误报为失败会导致前端显示错误，但 Profile 实际已切换。仅接受该类传输
	// 中断，不把其他 eUICC APDU 状态（例如 6A80）当作成功。
	if isAcceptedProfileSwitchInterruption(err) {
		return nil
	}
	return err
}

// isAcceptedProfileSwitchStatus 判断是否为 eUICC 已接受切换、但带有
// Proactive SIM Command 的 91xx 状态。91xx 不是 Profile 操作被拒绝，
// 而是模块即将执行 REFRESH；例如 910B 表示仍有 11 字节命令待读取。
func isAcceptedProfileSwitchStatus(err error) bool {
	return err != nil && strings.Contains(strings.ToUpper(err.Error()), "STATUS 91")
}

// isAcceptedProfileSwitchInterruption 判断 EnableProfile 已发出后，模块因
// SIM REFRESH 重置 APDU 通道导致的预期传输中断。这些错误只会在已通过
// isLikelyRefreshDisruption 过滤后使用，不能用于屏蔽普通 APDU 业务错误。
func isAcceptedProfileSwitchInterruption(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "CSIM: no response") ||
		strings.Contains(msg, "CSIM response too short") ||
		strings.Contains(msg, "decode CSIM response") ||
		strings.Contains(msg, "CME ERROR")
}

// isLikelyRefreshDisruption 判断 enable/disable 错误是否疑似 SIM REFRESH
// 打断了末条 APDU 响应（而非 eUICC 语义拒绝）。这类错误往往操作已成功，
// 应通过事后核验确认实际状态，而非直接抛给用户。
func isLikelyRefreshDisruption(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "unexpected response with status") || // 非常规 SW（如 910B）
		strings.Contains(msg, "CSIM response too short") ||
		strings.Contains(msg, "decode CSIM response") ||
		strings.Contains(msg, "CSIM: no response") ||
		strings.Contains(msg, "CME ERROR") ||
		strings.Contains(msg, "MANAGE CHANNEL")
}

// activeICCIDMatches 等待 SIM 复位完成后，用基础 AT 命令读取当前活动 ICCID，
// 判断是否已切换到目标 Profile。走 AT+CCID 等命令而非 ISD-R 通道，
// 避免 REFRESH 后逻辑通道耗尽导致无法核验。
func activeICCIDMatches(conn *ModemConn, iccid string, maxWait time.Duration) bool {
	target := normalizeCardIdentifier(iccid)
	if target == "" {
		return false
	}
	WaitForNetworkReady(conn, maxWait)
	for i := 0; i < 4; i++ {
		if active := readConnICCID(conn); active != "" && active == target {
			conn.SetICCID(active)
			return true
		}
		time.Sleep(800 * time.Millisecond)
	}
	return false
}

// DisableProfileViaLpa 禁用指定 ICCID 的 Profile。
// 与 Enable 共用 profileCommandViaLpa，同样具备 CAT busy 自动离线重试能力。
func DisableProfileViaLpa(conn *ModemConn, iccid string) error {
	defer GetModemService().InvalidateModemCache(conn.Name)
	return profileCommandViaLpa(conn, iccid, false)
}

// switchProfileClient 在已建立的 lpa 会话内切换 Profile（enable/disable），
// 复用调用方 client（不嵌套 withLpa），遇 CAT busy 自动离线重试。
//
// CAT busy 常见于“切换”场景：当前唯一启用 Profile 正被模块占用，
// eUICC 拒绝停用/切换。此时先将模块离线（AT+CFUN=0）释放该 Profile，
// 再重试原操作；无论结果都恢复射频（AT+CFUN=1），由调用方等待重新驻网。
// 供 Enable/Disable/Delete 复用。
func switchProfileClient(client *lpa.Client, conn *ModemConn, id sgp22.ICCID, enable bool) error {
	call := func() error {
		if enable {
			return client.EnableProfile(id, true)
		}
		return client.DisableProfile(id, true)
	}
	if err := call(); err != nil {
		if !strings.Contains(err.Error(), "cat busy") {
			return err
		}
		// CAT busy：先离线让模块释放当前启用 Profile，再重试；无论结果都恢复射频
		if _, e := conn.Device.SendCommandWithTimeout("AT+CFUN=0", 10*time.Second); e != nil {
			return fmt.Errorf("模块离线失败: %w", e)
		}
		time.Sleep(1500 * time.Millisecond)
		retryErr := call()
		conn.Device.SendCommandWithTimeout("AT+CFUN=1", 10*time.Second)
		return retryErr
	}
	return nil
}

// DeleteProfileViaLpa 删除指定 ICCID 的 Profile。
// 若 Profile 处于启用状态导致删除被拒，先禁用（处理 CAT busy 自动离线）再删除。
func DeleteProfileViaLpa(conn *ModemConn, iccid string) error {
	// 删除启用中的 Profile 会先禁用，同样需要失效设备信息缓存
	defer GetModemService().InvalidateModemCache(conn.Name)
	return withLpa(conn, func(client *lpa.Client) error {
		id, err := sgp22.NewICCID(iccid)
		if err != nil {
			return fmt.Errorf("无效 ICCID: %w", err)
		}
		if err := client.DeleteProfile(id); err != nil {
			// 启用状态的 Profile 不能直接删除，先禁用再删
			if !isProfileStateBlocked(err) {
				return fmt.Errorf("删除 Profile: %w", err)
			}
			if err := switchProfileClient(client, conn, id, false); err != nil {
				return fmt.Errorf("删除前禁用失败: %w", err)
			}
			if err := client.DeleteProfile(id); err != nil {
				return fmt.Errorf("删除 Profile: %w", err)
			}
		}
		return nil
	})
}

// isProfileStateBlocked 判断是否因 Profile 启用状态导致操作被拒（SGP.22 result code 2）。
func isProfileStateBlocked(err error) bool {
	msg := err.Error()
	return strings.Contains(msg, "not in enabled state") || strings.Contains(msg, "not in disabled state")
}

// stripNonDigits 只保留数字字符（IMEI 应为纯数字，过滤空格/+/- 等）。
func stripNonDigits(s string) string {
	return strings.Map(func(r rune) rune {
		if r >= '0' && r <= '9' {
			return r
		}
		return -1
	}, s)
}

// profileCommandViaLpa 是 enable/disable 的共用实现，均具备 CAT busy 自动离线重试。
func profileCommandViaLpa(conn *ModemConn, iccid string, enable bool) error {
	return withLpa(conn, func(client *lpa.Client) error {
		id, err := sgp22.NewICCID(iccid)
		if err != nil {
			return fmt.Errorf("无效 ICCID: %w", err)
		}
		if err := switchProfileClient(client, conn, id, enable); err != nil {
			if enable {
				return fmt.Errorf("启用 Profile: %w", err)
			}
			return fmt.Errorf("禁用 Profile: %w", err)
		}
		return nil
	})
}
