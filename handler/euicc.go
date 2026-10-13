package handler

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gorilla/mux"

	"github.com/rehiy/web-modem/database"
	"github.com/rehiy/web-modem/service"
)

// EuiccHandler handles eUICC/eSIM profile management endpoints.
type EuiccHandler struct {
	ms *service.ModemService
}

func NewEuiccHandler() *EuiccHandler {
	return &EuiccHandler{ms: service.GetModemService()}
}

// ListProfiles handles GET /api/euicc/profiles?name=<port>
func (h *EuiccHandler) ListProfiles(w http.ResponseWriter, r *http.Request) {
	name := r.URL.Query().Get("name")
	if name == "" {
		respondJSON(w, http.StatusBadRequest, H{"error": "name is required"})
		return
	}
	conn, err := h.ms.GetConn(name)
	if err != nil {
		respondJSON(w, http.StatusBadRequest, H{"error": err.Error()})
		return
	}
	info, err := service.GetEuiccInfoViaLpa(conn)
	if err != nil {
		respondJSON(w, http.StatusInternalServerError, H{"error": euiccErrMsg(err)})
		return
	}
	respondJSON(w, http.StatusOK, info)
}

// EnableProfile handles POST /api/euicc/profiles/{iccid}/enable
func (h *EuiccHandler) EnableProfile(w http.ResponseWriter, r *http.Request) {
	h.switchProfile(w, r, "enable")
}

// DisableProfile handles POST /api/euicc/profiles/{iccid}/disable
func (h *EuiccHandler) DisableProfile(w http.ResponseWriter, r *http.Request) {
	h.switchProfile(w, r, "disable")
}

func (h *EuiccHandler) switchProfile(w http.ResponseWriter, r *http.Request, action string) {
	var body struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Name == "" {
		body.Name = r.URL.Query().Get("name")
	}
	if body.Name == "" {
		respondJSON(w, http.StatusBadRequest, H{"error": "name is required"})
		return
	}

	iccid := mux.Vars(r)["iccid"]
	if iccid == "" {
		respondJSON(w, http.StatusBadRequest, H{"error": "iccid is required"})
		return
	}

	conn, err := h.ms.GetConn(body.Name)
	if err != nil {
		respondJSON(w, http.StatusBadRequest, H{"error": err.Error()})
		return
	}

	if action == "enable" {
		err = service.EnableProfileViaLpa(conn, iccid)
	} else {
		err = service.DisableProfileViaLpa(conn, iccid)
	}
	if err != nil {
		respondJSON(w, http.StatusInternalServerError, H{"error": euiccErrMsg(err)})
		return
	}

	// Wait for SIM to settle after profile switch, then return updated info.
	service.WaitForNetworkReady(conn, 8*time.Second)

	info, err := service.GetEuiccInfoViaLpa(conn)
	if err != nil {
		// Profile switch succeeded; return partial success even if re-read fails.
		respondJSON(w, http.StatusOK, H{"status": action + "d", "iccid": iccid})
		return
	}
	respondJSON(w, http.StatusOK, H{"status": action + "d", "iccid": iccid, "euicc": info})
}

// DeleteProfile handles DELETE /api/euicc/profiles/{iccid}
func (h *EuiccHandler) DeleteProfile(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name  string `json:"name"`
		Force bool   `json:"force"`
	}
	json.NewDecoder(r.Body).Decode(&body)
	if body.Name == "" {
		body.Name = r.URL.Query().Get("name")
	}
	if body.Name == "" {
		respondJSON(w, http.StatusBadRequest, H{"error": "name is required"})
		return
	}

	iccid := mux.Vars(r)["iccid"]
	if iccid == "" {
		respondJSON(w, http.StatusBadRequest, H{"error": "iccid is required"})
		return
	}

	conn, err := h.ms.GetConn(body.Name)
	if err != nil {
		respondJSON(w, http.StatusBadRequest, H{"error": err.Error()})
		return
	}

	// Guard: refuse to delete the only enabled profile unless force=true
	if !body.Force {
		info, err := service.GetEuiccInfoViaLpa(conn)
		if err == nil {
			enabledCount := 0
			isTarget := false
			for _, p := range info.Profiles {
				if p.IsEnabled {
					enabledCount++
					if p.ICCID == iccid {
						isTarget = true
					}
				}
			}
			if isTarget && enabledCount == 1 {
				respondJSON(w, http.StatusConflict, H{
					"error": "该 Profile 是唯一已启用的 Profile，请先切换到其他 Profile 或使用 force=true",
				})
				return
			}
		}
	}

	if err := service.DeleteProfileViaLpa(conn, iccid); err != nil {
		respondJSON(w, http.StatusInternalServerError, H{"error": euiccErrMsg(err)})
		return
	}
	// 同步清理该 Profile 的号码缓存
	_ = database.DeleteProfileNumber(iccid)
	respondJSON(w, http.StatusOK, H{"status": "deleted", "iccid": iccid})
}

// SetProfileNumber handles PUT /api/euicc/profiles/{iccid}/number
//
// 手动设置某个 Profile 的手机号（MSISDN）。模块无法读取未启用 Profile 的号码，
// 故提供手动补录入口；缓存按 ICCID 保存，Profile 列表展示时读取。
// 若该 ICCID 恰好是当前启用 Profile，同时刷新连接缓存，使顶栏立即显示新号码。
func (h *EuiccHandler) SetProfileNumber(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name   string `json:"name"`
		Number string `json:"number"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)
	if body.Name == "" {
		body.Name = r.URL.Query().Get("name")
	}

	iccid := mux.Vars(r)["iccid"]
	if iccid == "" {
		respondJSON(w, http.StatusBadRequest, H{"error": "iccid is required"})
		return
	}
	number := strings.TrimSpace(body.Number)
	if number == "" {
		respondJSON(w, http.StatusBadRequest, H{"error": "number is required"})
		return
	}

	if err := database.UpsertProfileNumber(iccid, number); err != nil {
		respondJSON(w, http.StatusInternalServerError, H{"error": err.Error()})
		return
	}

	// 若是当前启用 Profile，同步连接缓存，顶栏读取 /modem/info 时即可命中
	if body.Name != "" {
		if conn, err := h.ms.GetConn(body.Name); err == nil && conn.State().ICCID == iccid {
			conn.SetNumber(number)
			h.ms.InvalidateModemCache(conn.Name)
		}
	}
	respondJSON(w, http.StatusOK, H{"status": "ok", "iccid": iccid, "number": number})
}

// GetRotation handles GET /api/euicc/rotation?name=<port>
//
// 返回指定设备的 eUICC Profile 轮询配置（开关、间隔、主卡、上次运行时间）。
func (h *EuiccHandler) GetRotation(w http.ResponseWriter, r *http.Request) {
	name := r.URL.Query().Get("name")
	if name == "" {
		respondJSON(w, http.StatusBadRequest, H{"error": "name is required"})
		return
	}
	row, err := database.GetEuiccRotation(name)
	if err != nil {
		respondJSON(w, http.StatusInternalServerError, H{"error": err.Error()})
		return
	}
	respondJSON(w, http.StatusOK, row)
}

// SetRotation handles PUT /api/euicc/rotation
//
// 保存指定设备的轮询配置。intervalMin 小于最小值会被拒绝。
func (h *EuiccHandler) SetRotation(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name         string `json:"name"`
		Enabled      bool   `json:"enabled"`
		IntervalMin  int    `json:"intervalMin"`
		PrimaryICCID string `json:"primaryIccid"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		respondJSON(w, http.StatusBadRequest, H{"error": err.Error()})
		return
	}
	if body.Name == "" {
		respondJSON(w, http.StatusBadRequest, H{"error": "name is required"})
		return
	}
	if body.Enabled && body.IntervalMin < service.RotationMinIntervalMin {
		respondJSON(w, http.StatusBadRequest, H{"error": fmt.Sprintf("intervalMin 不能小于 %d 分钟", service.RotationMinIntervalMin)})
		return
	}

	row, _ := database.GetEuiccRotation(body.Name)
	row.ModemName = body.Name
	row.Enabled = body.Enabled
	row.IntervalMin = body.IntervalMin
	row.PrimaryICCID = strings.TrimSpace(body.PrimaryICCID)
	if err := database.SaveEuiccRotation(row); err != nil {
		respondJSON(w, http.StatusInternalServerError, H{"error": err.Error()})
		return
	}
	respondJSON(w, http.StatusOK, H{"status": "ok", "rotation": row})
}

// RunRotation handles POST /api/euicc/rotation/run?name=<port>
//
// 手动立即触发一轮轮询（异步执行，立即返回）。用于测试或不想等定时点。
func (h *EuiccHandler) RunRotation(w http.ResponseWriter, r *http.Request) {
	name := r.URL.Query().Get("name")
	if name == "" {
		respondJSON(w, http.StatusBadRequest, H{"error": "name is required"})
		return
	}
	go func() {
		if err := service.RunEuiccRotation(name); err != nil {
			log.Printf("[%s] manual euicc rotation failed: %v", name, err)
		}
	}()
	respondJSON(w, http.StatusOK, H{"status": "started", "name": name})
}

// CompareProfiles handles GET /api/euicc/compare?name=<port>
//
// 同时调用现有手写实现（GetEuiccInfo）与基于 euicc-go 的实现（GetEuiccInfoViaLpa），
// 返回两份结果及其错误，供真机对照验证 csimChannel 适配器的 APDU 透传路径。
// 任一实现失败不影响另一实现的返回，便于定位差异来源。
func (h *EuiccHandler) CompareProfiles(w http.ResponseWriter, r *http.Request) {
	name := r.URL.Query().Get("name")
	if name == "" {
		respondJSON(w, http.StatusBadRequest, H{"error": "name is required"})
		return
	}
	conn, err := h.ms.GetConn(name)
	if err != nil {
		respondJSON(w, http.StatusBadRequest, H{"error": err.Error()})
		return
	}

	// 两次会话各自持锁、开/关 ISD-R 通道，串行执行
	legacy, errLegacy := service.GetEuiccInfoViaLpa(conn)
	lpaInfo, errLpa := service.GetEuiccInfoViaLpa(conn)

	respondJSON(w, http.StatusOK, H{
		"legacy":      legacy,
		"lpa":         lpaInfo,
		"legacyError": euiccErrMsg(errLegacy),
		"lpaError":    euiccErrMsg(errLpa),
		"match": H{
			"eid":          legacy.EID != "" && legacy.EID == lpaInfo.EID,
			"profileCount": len(legacy.Profiles) == len(lpaInfo.Profiles),
		},
	})
}

// GetInfo2 handles GET /api/euicc/info2?name=<port>
// 读取 EUICCInfo2（BF20），返回完整 hex 及尽力解析的版本字段。
func (h *EuiccHandler) GetInfo2(w http.ResponseWriter, r *http.Request) {
	name := r.URL.Query().Get("name")
	if name == "" {
		respondJSON(w, http.StatusBadRequest, H{"error": "name is required"})
		return
	}
	conn, err := h.ms.GetConn(name)
	if err != nil {
		respondJSON(w, http.StatusBadRequest, H{"error": err.Error()})
		return
	}
	info, err := service.GetEuiccInfo2ViaLpa(conn)
	if err != nil {
		respondJSON(w, http.StatusInternalServerError, H{"error": euiccErrMsg(err)})
		return
	}
	respondJSON(w, http.StatusOK, info)
}

// GetAddresses handles GET /api/euicc/addresses?name=<port>
// 读取默认 SM-DP+ 与 SM-DS 地址。
func (h *EuiccHandler) GetAddresses(w http.ResponseWriter, r *http.Request) {
	name := r.URL.Query().Get("name")
	if name == "" {
		respondJSON(w, http.StatusBadRequest, H{"error": "name is required"})
		return
	}
	conn, err := h.ms.GetConn(name)
	if err != nil {
		respondJSON(w, http.StatusBadRequest, H{"error": err.Error()})
		return
	}
	addr, err := service.GetEuiccConfiguredAddressesViaLpa(conn)
	if err != nil {
		respondJSON(w, http.StatusInternalServerError, H{"error": euiccErrMsg(err)})
		return
	}
	respondJSON(w, http.StatusOK, addr)
}

// SetDefaultSMDP handles PUT /api/euicc/addresses?name=<port>  body: {"address":"..."}
// 设置默认 SM-DP+ 地址（下载 Profile 的前置配置）。
func (h *EuiccHandler) SetDefaultSMDP(w http.ResponseWriter, r *http.Request) {
	name := r.URL.Query().Get("name")
	var body struct {
		Name    string `json:"name"`
		Address string `json:"address"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)
	if name == "" {
		name = body.Name
	}
	if name == "" {
		respondJSON(w, http.StatusBadRequest, H{"error": "name is required"})
		return
	}
	if strings.TrimSpace(body.Address) == "" {
		respondJSON(w, http.StatusBadRequest, H{"error": "address is required"})
		return
	}
	conn, err := h.ms.GetConn(name)
	if err != nil {
		respondJSON(w, http.StatusBadRequest, H{"error": err.Error()})
		return
	}
	if err := service.SetDefaultSMDPAddressViaLpa(conn, strings.TrimSpace(body.Address)); err != nil {
		respondJSON(w, http.StatusInternalServerError, H{"error": euiccErrMsg(err)})
		return
	}
	respondJSON(w, http.StatusOK, H{"status": "ok", "address": body.Address})
}

// ListNotifications handles GET /api/euicc/notifications?name=<port>
// 列出本地待处理通知元数据。
func (h *EuiccHandler) ListNotifications(w http.ResponseWriter, r *http.Request) {
	name := r.URL.Query().Get("name")
	if name == "" {
		respondJSON(w, http.StatusBadRequest, H{"error": "name is required"})
		return
	}
	conn, err := h.ms.GetConn(name)
	if err != nil {
		respondJSON(w, http.StatusBadRequest, H{"error": err.Error()})
		return
	}
	list, err := service.ListEuiccNotificationsViaLpa(conn)
	if err != nil {
		respondJSON(w, http.StatusInternalServerError, H{"error": euiccErrMsg(err)})
		return
	}
	respondJSON(w, http.StatusOK, H{"notifications": list})
}

// HandleNotification handles POST /api/euicc/notifications/{seq}/handle?name=<port>
// 上报指定序号的待处理通知回执给 SM-DP+/SM-DS，成功后从本地列表删除。
func (h *EuiccHandler) HandleNotification(w http.ResponseWriter, r *http.Request) {
	name := r.URL.Query().Get("name")
	var body struct {
		Name string `json:"name"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)
	if name == "" {
		name = body.Name
	}
	if name == "" {
		respondJSON(w, http.StatusBadRequest, H{"error": "name is required"})
		return
	}
	seq, err := strconv.ParseInt(mux.Vars(r)["seq"], 10, 64)
	if err != nil || seq <= 0 {
		respondJSON(w, http.StatusBadRequest, H{"error": "invalid seq"})
		return
	}
	conn, err := h.ms.GetConn(name)
	if err != nil {
		respondJSON(w, http.StatusBadRequest, H{"error": err.Error()})
		return
	}
	if err := service.HandleEuiccNotificationViaLpa(conn, seq); err != nil {
		respondJSON(w, http.StatusInternalServerError, H{"error": euiccErrMsg(err)})
		return
	}
	respondJSON(w, http.StatusOK, H{"status": "handled", "seq": seq})
}

// DownloadProfile handles POST /api/euicc/download  body: {"name","activationCode"}
// 用激活码（LPA:1$...）下载并安装 Profile，IMEI 取自设备缓存（下载流程必需）。
// 同步执行：下载含多次 HTTPS 与多段 APDU，可能耗时 30s~2min。
func (h *EuiccHandler) DownloadProfile(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name           string `json:"name"`
		ActivationCode string `json:"activationCode"`
		IMEI           string `json:"imei"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		respondJSON(w, http.StatusBadRequest, H{"error": "请求体格式错误"})
		return
	}
	name := body.Name
	if name == "" {
		name = r.URL.Query().Get("name")
	}
	if name == "" {
		respondJSON(w, http.StatusBadRequest, H{"error": "name is required"})
		return
	}
	ac := strings.TrimSpace(body.ActivationCode)
	if ac == "" {
		respondJSON(w, http.StatusBadRequest, H{"error": "activationCode is required"})
		return
	}
	conn, err := h.ms.GetConn(name)
	if err != nil {
		respondJSON(w, http.StatusBadRequest, H{"error": err.Error()})
		return
	}
	if conn.State().IMEI == "" {
		respondJSON(w, http.StatusBadRequest, H{"error": "未读到设备 IMEI，下载流程需要 IMEI"})
		return
	}
	result, err := service.DownloadProfileViaLpa(conn, ac, body.IMEI)
	if err != nil {
		respondJSON(w, http.StatusInternalServerError, H{"error": euiccErrMsg(err)})
		return
	}
	respondJSON(w, http.StatusOK, H{"status": "downloaded", "result": result})
}

// euiccErrMsg translates common APDU/eUICC errors into readable Chinese messages.
func euiccErrMsg(err error) string {
	if err == nil {
		return ""
	}
	msg := err.Error()
	switch {
	// 先匹配具体 SW 状态码，再回退到通用错误
	case strings.Contains(msg, "SW=6A81"):
		return "逻辑通道已耗尽，请复位模块（AT+CFUN=1,1）或重新插拔后重试"
	case strings.Contains(msg, "SW=6A82") || strings.Contains(msg, "SW=6B00"):
		return "ISD-R AID 不存在，该 SIM 不是 eUICC（普通 SIM 卡）"
	case strings.Contains(msg, "SW=6985"):
		return "eUICC 状态不允许此操作（权限或锁定）"
	case strings.Contains(msg, "SW=6982"):
		return "安全状态不满足"
	case strings.Contains(msg, "SW=6300"):
		return "Profile 切换失败（未找到目标 ICCID）"
	case strings.Contains(msg, "MANAGE CHANNEL"):
		return "无法打开逻辑通道，模块固件可能不支持 AT+CSIM 通道操作"
	case strings.Contains(msg, "CSIM: no response") || strings.Contains(msg, "CME ERROR"):
		return "AT+CSIM 命令被模块拒绝，请确认当前串口支持 APDU 透传"
	case strings.Contains(msg, "open ISD-R"):
		return "无法打开 ISD-R 逻辑通道，请确认所选串口的 SIM 是 eUICC 且已插入"
	case strings.Contains(msg, "APDU error"):
		return "eUICC 返回错误：" + msg
	case strings.Contains(msg, "not connected"):
		return "设备未连接"
	case strings.Contains(msg, "cat busy"):
		return "eUICC 正忙（CAT）：该 Profile 正被模块使用，无法直接停用/切换。请改为启用另一张 Profile（自动切换），或先将模块离线（AT+CFUN=0）后重试"
	case strings.Contains(strings.ToLower(msg), "busy"):
		return "模块或 eUICC 正忙，请稍后重试"
	default:
		return msg
	}
}
