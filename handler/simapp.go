package handler

import (
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode/utf16"
	"unicode/utf8"

	"github.com/rehiy/web-modem/service"
)

type SimAppHandler struct {
	ms *service.ModemService
}

type atResult struct {
	OK          bool     `json:"ok"`
	Unsupported bool     `json:"unsupported,omitempty"`
	Error       string   `json:"error,omitempty"`
	Raw         []string `json:"raw"`
}

type simCapability struct {
	Key         string   `json:"key"`
	Label       string   `json:"label"`
	Command     string   `json:"command"`
	Supported   bool     `json:"supported"`
	Error       string   `json:"error,omitempty"`
	Raw         []string `json:"raw"`
	Description string   `json:"description"`
}

type pdpContext struct {
	CID     int    `json:"cid"`
	PDPType string `json:"pdp_type"`
	APN     string `json:"apn"`
	State   *int   `json:"state,omitempty"`
	IP      string `json:"ip,omitempty"`
	Raw     string `json:"raw,omitempty"`
}

type simFileInfo struct {
	Key         string `json:"key"`
	Label       string `json:"label"`
	FileID      string `json:"file_id"`
	SW1         string `json:"sw1,omitempty"`
	SW2         string `json:"sw2,omitempty"`
	Hex         string `json:"hex,omitempty"`
	Decoded     string `json:"decoded,omitempty"`
	Error       string `json:"error,omitempty"`
	Unsupported bool   `json:"unsupported,omitempty"`
}

type networkOperator struct {
	Status      int    `json:"status"`
	StatusLabel string `json:"status_label"`
	LongName    string `json:"long_name"`
	ShortName   string `json:"short_name"`
	Numeric     string `json:"numeric"`
	Act         int    `json:"act"`
	ActLabel    string `json:"act_label"`
	Raw         string `json:"raw"`
}

type phonebookEntry struct {
	Index  int    `json:"index"`
	Number string `json:"number"`
	Type   int    `json:"type"`
	Name   string `json:"name"`
	Raw    string `json:"raw,omitempty"`
}

type phonebookInfo struct {
	Storage string           `json:"storage"`
	Range   string           `json:"range"`
	Entries []phonebookEntry `json:"entries"`
	Raw     []string         `json:"raw"`
}

type ussdMessage struct {
	Status  string `json:"status,omitempty"`
	Message string `json:"message,omitempty"`
	Decoded string `json:"decoded,omitempty"`
	DCS     string `json:"dcs,omitempty"`
	Raw     string `json:"raw,omitempty"`
}

func NewSimAppHandler() *SimAppHandler {
	return &SimAppHandler{
		ms: service.GetModemService(),
	}
}

func (h *SimAppHandler) GetCapabilities(w http.ResponseWriter, r *http.Request) {
	conn, ok := h.requireConn(w, r.URL.Query().Get("name"))
	if !ok {
		return
	}

	probes := []struct {
		key         string
		label       string
		command     string
		description string
	}{
		{"ussd", "USSD", "AT+CUSD=?", "运营商服务码交互"},
		{"stk_standard", "标准 STK", "AT+STIN?", "SIM Toolkit 菜单通知"},
		{"stk_menu", "STK 菜单读取", "AT+STGI=?", "读取 SIM 应用菜单"},
		{"stk_select", "STK 菜单选择", "AT+STGR=?", "选择 SIM 应用菜单项"},
		{"stk_quectel", "Quectel STK", "AT+QSTK=?", "移远/兼容 STK 指令族探测"},
		{"apdu", "APDU", "AT+CSIM=?", "通用 SIM APDU 通道"},
		{"crsm", "CRSM", "AT+CRSM=?", "受限 SIM 文件访问"},
		{"pin", "PIN 管理", "AT+CPIN?", "SIM PIN/PUK 状态与验证"},
		{"pdp", "APN/PDP", "AT+CGDCONT?", "蜂窝数据上下文配置"},
	}

	caps := []simCapability{}
	for _, probe := range probes {
		raw, err := conn.SendCommandWithTimeout(probe.command, 3*time.Second)
		capability := simCapability{
			Key:         probe.key,
			Label:       probe.label,
			Command:     probe.command,
			Supported:   atCommandSupported(raw, err),
			Raw:         raw,
			Description: probe.description,
		}
		if err != nil {
			capability.Error = err.Error()
		} else if atResponseHasFinalError(raw) {
			capability.Error = finalErrorLine(raw)
		}
		caps = append(caps, capability)
	}

	respondJSON(w, http.StatusOK, H{"ok": true, "data": caps})
}

func (h *SimAppHandler) SendUSSD(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name string `json:"name"`
		Code string `json:"code"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	conn, ok := h.requireConn(w, req.Name)
	if !ok {
		return
	}
	code := strings.TrimSpace(req.Code)
	if code == "" {
		respondJSON(w, http.StatusBadRequest, H{"error": "code is empty"})
		return
	}

	urcCh, cancel := h.ms.WatchURC(conn.Name)
	defer cancel()

	cmd := fmt.Sprintf("AT+CUSD=1,%s,15", strconv.Quote(code))
	raw, err := conn.SendCommandWithTimeout(cmd, 8*time.Second)
	result := atResponseResult(raw, err)
	resultData := H{"command": cmd, "result": result}

	if urc, ok := waitForURC(urcCh, 25*time.Second, "+CUSD"); ok {
		resultData["urc"] = urcToJSON(urc)
		message := ussdMessageFromURC(urc)
		resultData["ussd"] = message
		resultData["message"] = firstNonEmpty(message.Decoded, message.Message)
		resultData["status"] = message.Status
	} else if message, ok := firstUSSDMessage(raw); ok {
		resultData["ussd"] = message
		resultData["message"] = firstNonEmpty(message.Decoded, message.Message)
		resultData["status"] = message.Status
	}
	respondJSON(w, http.StatusOK, H{"ok": err == nil && !result.Unsupported, "data": resultData, "raw": raw})
}

func (h *SimAppHandler) CancelUSSD(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name string `json:"name"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	conn, ok := h.requireConn(w, req.Name)
	if !ok {
		return
	}
	raw, err := conn.SendCommandWithTimeout("AT+CUSD=2", 5*time.Second)
	respondJSON(w, statusForErr(err), H{"ok": err == nil && !atResponseHasFinalError(raw), "result": atResponseResult(raw, err), "raw": raw})
}

func (h *SimAppHandler) StartSTKSession(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name string `json:"name"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	conn, ok := h.requireConn(w, req.Name)
	if !ok {
		return
	}

	commands := []string{"AT+STIN?", "AT+STGI=0", "AT+QSTK?"}
	results := []H{}
	for _, cmd := range commands {
		raw, err := conn.SendCommandWithTimeout(cmd, 5*time.Second)
		results = append(results, H{
			"command": cmd,
			"result":  atResponseResult(raw, err),
			"parsed":  parseSTKResponse(raw),
		})
		if atCommandSupported(raw, err) {
			break
		}
	}

	respondJSON(w, http.StatusOK, H{"ok": true, "data": results})
}

func (h *SimAppHandler) SelectSTKItem(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name    string `json:"name"`
		ItemID  int    `json:"item_id"`
		Command string `json:"command"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	conn, ok := h.requireConn(w, req.Name)
	if !ok {
		return
	}
	// 该操作可能改变 SIM/注册状态，完成后使设备信息缓存失效
	defer h.ms.InvalidateModemCache(conn.Name)
	cmd := strings.TrimSpace(req.Command)
	if cmd == "" {
		if req.ItemID <= 0 {
			respondJSON(w, http.StatusBadRequest, H{"error": "item_id is required"})
			return
		}
		cmd = fmt.Sprintf("AT+STGR=0,1,%d", req.ItemID)
	}

	urcCh, cancel := h.ms.WatchURC(conn.Name)
	defer cancel()

	raw, err := conn.SendCommandWithTimeout(cmd, 8*time.Second)
	data := H{"command": cmd, "result": atResponseResult(raw, err), "parsed": parseSTKResponse(raw)}
	if urc, ok := waitForURC(urcCh, 10*time.Second, "+STIN", "+STGI", "+QSTK"); ok {
		data["urc"] = urcToJSON(urc)
	}
	respondJSON(w, http.StatusOK, H{"ok": err == nil && !atResponseHasFinalError(raw), "data": data, "raw": raw})
}

func (h *SimAppHandler) RespondSTK(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name    string `json:"name"`
		Text    string `json:"text"`
		Command string `json:"command"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	conn, ok := h.requireConn(w, req.Name)
	if !ok {
		return
	}
	// 该操作可能改变 SIM/注册状态，完成后使设备信息缓存失效
	defer h.ms.InvalidateModemCache(conn.Name)
	cmd := strings.TrimSpace(req.Command)
	if cmd == "" {
		cmd = fmt.Sprintf("AT+STGR=3,1,%s", strconv.Quote(req.Text))
	}

	urcCh, cancel := h.ms.WatchURC(conn.Name)
	defer cancel()

	raw, err := conn.SendCommandWithTimeout(cmd, 8*time.Second)
	data := H{"command": cmd, "result": atResponseResult(raw, err), "parsed": parseSTKResponse(raw)}
	if urc, ok := waitForURC(urcCh, 10*time.Second, "+STIN", "+STGI", "+QSTK"); ok {
		data["urc"] = urcToJSON(urc)
	}
	respondJSON(w, http.StatusOK, H{"ok": err == nil && !atResponseHasFinalError(raw), "data": data, "raw": raw})
}

func (h *SimAppHandler) VerifyPIN(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name string `json:"name"`
		PIN  string `json:"pin"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	conn, ok := h.requireConn(w, req.Name)
	if !ok {
		return
	}
	// 该操作可能改变 SIM/注册状态，完成后使设备信息缓存失效
	defer h.ms.InvalidateModemCache(conn.Name)
	if strings.TrimSpace(req.PIN) == "" {
		respondJSON(w, http.StatusBadRequest, H{"error": "pin is empty"})
		return
	}
	err := conn.VerifyPIN(req.PIN)
	respondOperation(w, err)
}

func (h *SimAppHandler) ChangePIN(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name   string `json:"name"`
		OldPIN string `json:"old_pin"`
		NewPIN string `json:"new_pin"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	conn, ok := h.requireConn(w, req.Name)
	if !ok {
		return
	}
	if strings.TrimSpace(req.OldPIN) == "" || strings.TrimSpace(req.NewPIN) == "" {
		respondJSON(w, http.StatusBadRequest, H{"error": "old_pin and new_pin are required"})
		return
	}
	respondOperation(w, conn.ChangePIN(req.OldPIN, req.NewPIN))
}

func (h *SimAppHandler) SetPINLock(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name     string `json:"name"`
		PINType  string `json:"pin_type"`
		Enable   bool   `json:"enable"`
		Password string `json:"password"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	conn, ok := h.requireConn(w, req.Name)
	if !ok {
		return
	}
	pinType := strings.TrimSpace(req.PINType)
	if pinType == "" {
		pinType = "SC"
	}
	if strings.TrimSpace(req.Password) == "" {
		respondJSON(w, http.StatusBadRequest, H{"error": "password is empty"})
		return
	}
	respondOperation(w, conn.UnlockPIN(pinType, req.Enable, req.Password))
}

func (h *SimAppHandler) UnlockPUK(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name   string `json:"name"`
		PUK    string `json:"puk"`
		NewPIN string `json:"new_pin"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	conn, ok := h.requireConn(w, req.Name)
	if !ok {
		return
	}
	// 该操作可能改变 SIM/注册状态，完成后使设备信息缓存失效
	defer h.ms.InvalidateModemCache(conn.Name)
	if strings.TrimSpace(req.PUK) == "" || strings.TrimSpace(req.NewPIN) == "" {
		respondJSON(w, http.StatusBadRequest, H{"error": "puk and new_pin are required"})
		return
	}
	cmd := fmt.Sprintf("AT+CPIN=%s,%s", strconv.Quote(req.PUK), strconv.Quote(req.NewPIN))
	raw, err := conn.SendCommandWithTimeout(cmd, 8*time.Second)
	respondJSON(w, statusForErr(err), H{"ok": err == nil && !atResponseHasFinalError(raw), "result": atResponseResult(raw, err), "raw": raw})
}

func (h *SimAppHandler) ListNetworkContexts(w http.ResponseWriter, r *http.Request) {
	conn, ok := h.requireConn(w, r.URL.Query().Get("name"))
	if !ok {
		return
	}
	contexts, raw, err := queryPDPContexts(conn)
	respondJSON(w, statusForErr(err), H{"ok": err == nil, "data": contexts, "raw": raw, "error": errorString(err)})
}

func (h *SimAppHandler) SetNetworkContext(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name    string `json:"name"`
		CID     int    `json:"cid"`
		PDPType string `json:"pdp_type"`
		APN     string `json:"apn"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	conn, ok := h.requireConn(w, req.Name)
	if !ok {
		return
	}
	if req.CID <= 0 || strings.TrimSpace(req.PDPType) == "" {
		respondJSON(w, http.StatusBadRequest, H{"error": "cid and pdp_type are required"})
		return
	}
	respondOperation(w, conn.SetAPN(req.CID, strings.ToUpper(strings.TrimSpace(req.PDPType)), strings.TrimSpace(req.APN)))
}

func (h *SimAppHandler) ActivateNetworkContext(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name  string `json:"name"`
		CID   int    `json:"cid"`
		State int    `json:"state"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	conn, ok := h.requireConn(w, req.Name)
	if !ok {
		return
	}
	// 该操作可能改变 SIM/注册状态，完成后使设备信息缓存失效
	defer h.ms.InvalidateModemCache(conn.Name)
	if req.CID <= 0 || (req.State != 0 && req.State != 1) {
		respondJSON(w, http.StatusBadRequest, H{"error": "cid and state 0/1 are required"})
		return
	}
	respondOperation(w, conn.SetPDPContext(req.CID, req.State))
}

func (h *SimAppHandler) GetNetworkIP(w http.ResponseWriter, r *http.Request) {
	conn, ok := h.requireConn(w, r.URL.Query().Get("name"))
	if !ok {
		return
	}
	cid, _ := strconv.Atoi(r.URL.Query().Get("cid"))
	raw, err := conn.SendCommandWithTimeout("AT+CGPADDR?", 5*time.Second)
	ips := []pdpContext{}
	if err == nil {
		ips = parseCGPADDR(raw, cid)
	}
	respondJSON(w, statusForErr(err), H{"ok": err == nil, "data": ips, "raw": raw, "error": errorString(err)})
}

func (h *SimAppHandler) ListOperators(w http.ResponseWriter, r *http.Request) {
	conn, ok := h.requireConn(w, r.URL.Query().Get("name"))
	if !ok {
		return
	}
	raw, err := conn.SendCommandWithTimeout("AT+COPS=?", 90*time.Second)
	operators := []networkOperator{}
	if err == nil {
		operators = parseCOPSOperators(raw)
	}
	respondJSON(w, statusForErr(err), H{"ok": err == nil && !atResponseHasFinalError(raw), "data": operators, "raw": raw, "error": errorString(err)})
}

func (h *SimAppHandler) SelectOperator(w http.ResponseWriter, r *http.Request) {
	// 字段对齐 AT+COPS 原生参数，与前端契约一致：
	// mode: 0=自动 1=手动 2=注销 4=手动优先
	// format: 0=长名称 1=短名称 2=数字
	// operator: 运营商，按 format 解释；act 为可选接入技术
	var req struct {
		Name     string `json:"name"`
		Mode     int    `json:"mode"`
		Format   int    `json:"format"`
		Operator string `json:"operator"`
		Act      *int   `json:"act"` // nil 表示不指定接入技术，区分未传与合法值 0
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	conn, ok := h.requireConn(w, req.Name)
	if !ok {
		return
	}
	// 该操作可能改变 SIM/注册状态，完成后使设备信息缓存失效
	defer h.ms.InvalidateModemCache(conn.Name)

	cmd, err := buildCOPSCommand(req.Mode, req.Format, req.Operator, req.Act)
	if err != nil {
		respondJSON(w, http.StatusBadRequest, H{"error": err.Error()})
		return
	}

	raw, err := conn.SendCommandWithTimeout(cmd, 60*time.Second)
	respondJSON(w, statusForErr(err), H{"ok": err == nil && !atResponseHasFinalError(raw), "command": cmd, "result": atResponseResult(raw, err), "raw": raw})
}

// buildCOPSCommand 根据 AT+COPS 参数构造选网命令。
// mode=0 自动、mode=2 注销；mode=1/4 为手动（含手动优先），需要 operator。
// format=2（数字）时 operator 规范化为纯数字，字母名称原样保留。
func buildCOPSCommand(mode, format int, operator string, act *int) (string, error) {
	switch mode {
	case 0: // 自动选网
		return "AT+COPS=0", nil
	case 2: // 注销网络
		return "AT+COPS=2", nil
	case 1, 4: // 手动 / 手动优先
		oper := strings.TrimSpace(operator)
		if format == 2 {
			oper = normalizeDigits(oper)
		}
		if oper == "" {
			return "", fmt.Errorf("operator is required for manual selection")
		}
		if act != nil {
			return fmt.Sprintf("AT+COPS=%d,%d,%s,%d", mode, format, strconv.Quote(oper), *act), nil
		}
		return fmt.Sprintf("AT+COPS=%d,%d,%s", mode, format, strconv.Quote(oper)), nil
	default:
		return "", fmt.Errorf("unsupported mode %d, must be 0, 1, 2 or 4", mode)
	}
}

func (h *SimAppHandler) ListPhonebook(w http.ResponseWriter, r *http.Request) {
	conn, ok := h.requireConn(w, r.URL.Query().Get("name"))
	if !ok {
		return
	}
	storage := sanitizePhonebookStorage(r.URL.Query().Get("storage"))
	rawSet, err := conn.SendCommandWithTimeout(fmt.Sprintf("AT+CPBS=%s", strconv.Quote(storage)), 5*time.Second)
	if err != nil || atResponseHasFinalError(rawSet) {
		respondJSON(w, statusForErr(err), H{"ok": false, "error": firstNonEmpty(errorString(err), finalErrorLine(rawSet)), "raw": rawSet})
		return
	}

	rawRange, _ := conn.SendCommandWithTimeout("AT+CPBR=?", 5*time.Second)
	start, end, rangeLabel := parseCPBRRange(rawRange)
	if start <= 0 || end < start {
		start, end = 1, 250
	}
	rawEntries, err := conn.SendCommandWithTimeout(fmt.Sprintf("AT+CPBR=%d,%d", start, end), 20*time.Second)
	raw := append([]string{}, rawSet...)
	raw = append(raw, rawRange...)
	raw = append(raw, rawEntries...)
	info := phonebookInfo{
		Storage: storage,
		Range:   rangeLabel,
		Entries: parseCPBREntries(rawEntries),
		Raw:     raw,
	}
	respondJSON(w, statusForErr(err), H{"ok": err == nil && !atResponseHasFinalError(rawEntries), "data": info, "raw": raw, "error": errorString(err)})
}

func (h *SimAppHandler) WritePhonebook(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name    string `json:"name"`
		Storage string `json:"storage"`
		Index   int    `json:"index"`
		Number  string `json:"number"`
		Type    int    `json:"type"`
		Label   string `json:"label"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	conn, ok := h.requireConn(w, req.Name)
	if !ok {
		return
	}
	storage := sanitizePhonebookStorage(req.Storage)
	if req.Index <= 0 || strings.TrimSpace(req.Number) == "" {
		respondJSON(w, http.StatusBadRequest, H{"error": "index and number are required"})
		return
	}
	numberType := req.Type
	if numberType == 0 {
		numberType = 129
	}
	rawSet, err := conn.SendCommandWithTimeout(fmt.Sprintf("AT+CPBS=%s", strconv.Quote(storage)), 5*time.Second)
	if err != nil || atResponseHasFinalError(rawSet) {
		respondJSON(w, statusForErr(err), H{"ok": false, "error": firstNonEmpty(errorString(err), finalErrorLine(rawSet)), "raw": rawSet})
		return
	}
	cmd := fmt.Sprintf("AT+CPBW=%d,%s,%d,%s", req.Index, strconv.Quote(strings.TrimSpace(req.Number)), numberType, strconv.Quote(strings.TrimSpace(req.Label)))
	raw, err := conn.SendCommandWithTimeout(cmd, 8*time.Second)
	raw = append(rawSet, raw...)
	respondJSON(w, statusForErr(err), H{"ok": err == nil && !atResponseHasFinalError(raw), "command": cmd, "result": atResponseResult(raw, err), "raw": raw})
}

func (h *SimAppHandler) DeletePhonebook(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name    string `json:"name"`
		Storage string `json:"storage"`
		Index   int    `json:"index"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	conn, ok := h.requireConn(w, req.Name)
	if !ok {
		return
	}
	storage := sanitizePhonebookStorage(req.Storage)
	if req.Index <= 0 {
		respondJSON(w, http.StatusBadRequest, H{"error": "index is required"})
		return
	}
	rawSet, err := conn.SendCommandWithTimeout(fmt.Sprintf("AT+CPBS=%s", strconv.Quote(storage)), 5*time.Second)
	if err != nil || atResponseHasFinalError(rawSet) {
		respondJSON(w, statusForErr(err), H{"ok": false, "error": firstNonEmpty(errorString(err), finalErrorLine(rawSet)), "raw": rawSet})
		return
	}
	cmd := fmt.Sprintf("AT+CPBW=%d", req.Index)
	raw, err := conn.SendCommandWithTimeout(cmd, 8*time.Second)
	raw = append(rawSet, raw...)
	respondJSON(w, statusForErr(err), H{"ok": err == nil && !atResponseHasFinalError(raw), "command": cmd, "result": atResponseResult(raw, err), "raw": raw})
}

// SetOwnNumber 设置本机号码（写入 SIM 的 ON 存储AT+CPBS="ON" + AT+CPBW），
// 供 AT+CNUM / 控制台显示。号码格式由模块处理，比手搓 MSISDN BCD 更稳妥。
func (h *SimAppHandler) SetOwnNumber(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name   string `json:"name"`
		Number string `json:"number"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	conn, ok := h.requireConn(w, req.Name)
	if !ok {
		return
	}
	number := strings.TrimSpace(req.Number)
	if number == "" {
		respondJSON(w, http.StatusBadRequest, H{"error": "number is required"})
		return
	}
	rawSet, err := conn.SendCommandWithTimeout(fmt.Sprintf("AT+CPBS=%s", strconv.Quote("ON")), 5*time.Second)
	if err != nil || atResponseHasFinalError(rawSet) {
		respondJSON(w, statusForErr(err), H{"ok": false, "error": firstNonEmpty(errorString(err), finalErrorLine(rawSet)), "raw": rawSet})
		return
	}
	numberType := 129
	if strings.HasPrefix(number, "+") {
		numberType = 145
	}
	cmd := fmt.Sprintf("AT+CPBW=1,%s,%d,%s", strconv.Quote(number), numberType, strconv.Quote(""))
	raw, err := conn.SendCommandWithTimeout(cmd, 8*time.Second)
	raw = append(rawSet, raw...)
	if err != nil || atResponseHasFinalError(raw) {
		respondJSON(w, statusForErr(err), H{"ok": false, "error": firstNonEmpty(errorString(err), finalErrorLine(raw)), "command": cmd, "raw": raw})
		return
	}
	conn.SetNumber(number)
	h.ms.InvalidateModemCache(conn.Name)
	respondJSON(w, http.StatusOK, H{"ok": true, "number": number})
}

func (h *SimAppHandler) ReadSIMFiles(w http.ResponseWriter, r *http.Request) {
	conn, ok := h.requireConn(w, r.URL.Query().Get("name"))
	if !ok {
		return
	}
	files := []struct {
		key    string
		label  string
		fileID int
		length int
		decode func(string) string
	}{
		{"iccid", "ICCID", 0x2FE2, 10, decodeICCID},
		{"spn", "SPN", 0x6F46, 17, decodeASCIIHex},
		{"msisdn", "MSISDN", 0x6F40, 30, decodeLooseBCD},
		{"ad", "Administrative Data", 0x6FAD, 4, strings.ToUpper},
		{"imsi", "IMSI", 0x6F07, 9, decodeIMSI},
	}

	result := []simFileInfo{}
	for _, file := range files {
		cmd := fmt.Sprintf("AT+CRSM=176,%d,0,0,%d", file.fileID, file.length)
		raw, err := conn.SendCommandWithTimeout(cmd, 5*time.Second)
		info := simFileInfo{
			Key:    file.key,
			Label:  file.label,
			FileID: fmt.Sprintf("0x%04X", file.fileID),
		}
		if err != nil {
			info.Error = err.Error()
		}
		fields, found := firstATFields(raw, "+CRSM")
		if !found || len(fields) < 2 || atResponseHasFinalError(raw) {
			info.Unsupported = true
			if info.Error == "" {
				info.Error = finalErrorLine(raw)
			}
			result = append(result, info)
			continue
		}
		info.SW1 = fields[0]
		info.SW2 = fields[1]
		if len(fields) > 2 {
			info.Hex = strings.Trim(fields[2], `"`)
			info.Decoded = file.decode(info.Hex)
		}
		result = append(result, info)
	}

	respondJSON(w, http.StatusOK, H{"ok": true, "data": result})
}

func (h *SimAppHandler) SendAPDU(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name string `json:"name"`
		APDU string `json:"apdu"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	conn, ok := h.requireConn(w, req.Name)
	if !ok {
		return
	}
	// 该操作可能改变 SIM/注册状态，完成后使设备信息缓存失效
	defer h.ms.InvalidateModemCache(conn.Name)
	apdu := cleanHex(req.APDU)
	if apdu == "" || len(apdu)%2 != 0 {
		respondJSON(w, http.StatusBadRequest, H{"error": "apdu must be an even-length hex string"})
		return
	}
	cmd := fmt.Sprintf("AT+CSIM=%d,%s", len(apdu), strconv.Quote(strings.ToUpper(apdu)))
	raw, err := conn.SendCommandWithTimeout(cmd, 10*time.Second)
	respondJSON(w, statusForErr(err), H{"ok": err == nil && !atResponseHasFinalError(raw), "command": cmd, "data": parseCSIMResponse(raw), "result": atResponseResult(raw, err), "raw": raw})
}

func (h *SimAppHandler) requireConn(w http.ResponseWriter, name string) (*service.ModemConn, bool) {
	name = strings.TrimSpace(name)
	if name == "" {
		respondJSON(w, http.StatusBadRequest, H{"error": "name is empty"})
		return nil, false
	}
	conn, err := h.ms.GetConn(name)
	if err != nil {
		respondJSON(w, http.StatusBadRequest, H{"error": err.Error()})
		return nil, false
	}
	return conn, true
}

func decodeJSON(w http.ResponseWriter, r *http.Request, dst any) bool {
	if err := json.NewDecoder(r.Body).Decode(dst); err != nil {
		respondJSON(w, http.StatusBadRequest, H{"error": err.Error()})
		return false
	}
	return true
}

func respondOperation(w http.ResponseWriter, err error) {
	if err != nil {
		respondJSON(w, http.StatusInternalServerError, H{"ok": false, "error": err.Error()})
		return
	}
	respondJSON(w, http.StatusOK, H{"ok": true})
}

func statusForErr(err error) int {
	if err != nil {
		return http.StatusInternalServerError
	}
	return http.StatusOK
}

func errorString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func atResponseResult(raw []string, err error) atResult {
	result := atResult{
		OK:          err == nil && !atResponseHasFinalError(raw),
		Unsupported: !atCommandSupported(raw, err),
		Raw:         raw,
	}
	if err != nil {
		result.Error = err.Error()
	} else if atResponseHasFinalError(raw) {
		result.Error = finalErrorLine(raw)
	}
	return result
}

func atCommandSupported(raw []string, err error) bool {
	if err != nil && len(raw) == 0 {
		return false
	}
	return !atResponseHasFinalError(raw)
}

func atResponseHasFinalError(raw []string) bool {
	for _, line := range raw {
		line = strings.ToUpper(strings.TrimSpace(line))
		if line == "ERROR" || strings.HasPrefix(line, "+CME ERROR") || strings.HasPrefix(line, "+CMS ERROR") {
			return true
		}
	}
	return false
}

func finalErrorLine(raw []string) string {
	for _, line := range raw {
		line = strings.TrimSpace(line)
		upper := strings.ToUpper(line)
		if upper == "ERROR" || strings.HasPrefix(upper, "+CME ERROR") || strings.HasPrefix(upper, "+CMS ERROR") {
			return line
		}
	}
	return ""
}

func waitForURC(ch <-chan service.ModemURC, timeout time.Duration, prefixes ...string) (service.ModemURC, bool) {
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	for {
		select {
		case urc := <-ch:
			if service.URCMatches(urc.Event, prefixes...) {
				return urc, true
			}
		case <-timer.C:
			return service.ModemURC{}, false
		}
	}
}

func urcToJSON(urc service.ModemURC) H {
	return H{
		"modem_name": urc.ModemName,
		"event":      urc.Event,
		"params":     urc.Params,
	}
}

func parseSTKResponse(raw []string) H {
	parsed := H{"menu": []H{}, "events": []H{}}
	menu := []H{}
	events := []H{}
	for _, line := range raw {
		label, fields, ok := parseATLine(line)
		if !ok {
			continue
		}
		switch label {
		case "+STIN":
			if len(fields) > 0 {
				events = append(events, H{"type": "STIN", "code": fields[0], "raw": line})
			}
		case "+STGI":
			item := H{"raw": line, "fields": fields}
			if len(fields) > 0 {
				item["id"] = intField(fields[0])
			}
			if len(fields) > 1 {
				item["text"] = strings.Trim(fields[1], `"`)
			}
			menu = append(menu, item)
		case "+QSTK":
			events = append(events, H{"type": "QSTK", "fields": fields, "raw": line})
		}
	}
	parsed["menu"] = menu
	parsed["events"] = events
	return parsed
}

func queryPDPContexts(conn *service.ModemConn) ([]pdpContext, []string, error) {
	rawAPN, err := conn.SendCommandWithTimeout("AT+CGDCONT?", 5*time.Second)
	if err != nil {
		return nil, rawAPN, err
	}
	contexts := parseCGDCONT(rawAPN)

	rawAct, _ := conn.SendCommandWithTimeout("AT+CGACT?", 5*time.Second)
	states := parseCGACT(rawAct)
	rawIP, _ := conn.SendCommandWithTimeout("AT+CGPADDR?", 5*time.Second)
	ips := parseCGPADDR(rawIP, 0)
	ipByCID := map[int]string{}
	for _, ip := range ips {
		ipByCID[ip.CID] = ip.IP
	}
	for i := range contexts {
		if state, ok := states[contexts[i].CID]; ok {
			contexts[i].State = &state
		}
		contexts[i].IP = ipByCID[contexts[i].CID]
	}

	raw := append([]string{}, rawAPN...)
	raw = append(raw, rawAct...)
	raw = append(raw, rawIP...)
	return contexts, raw, nil
}

func parseCGDCONT(raw []string) []pdpContext {
	contexts := []pdpContext{}
	for _, line := range raw {
		label, fields, ok := parseATLine(line)
		if !ok || label != "+CGDCONT" || len(fields) < 3 {
			continue
		}
		contexts = append(contexts, pdpContext{
			CID:     intField(fields[0]),
			PDPType: strings.Trim(fields[1], `"`),
			APN:     strings.Trim(fields[2], `"`),
			Raw:     line,
		})
	}
	return contexts
}

func parseCGACT(raw []string) map[int]int {
	states := map[int]int{}
	for _, line := range raw {
		label, fields, ok := parseATLine(line)
		if !ok || label != "+CGACT" || len(fields) < 2 {
			continue
		}
		states[intField(fields[0])] = intField(fields[1])
	}
	return states
}

func parseCGPADDR(raw []string, cid int) []pdpContext {
	contexts := []pdpContext{}
	for _, line := range raw {
		label, fields, ok := parseATLine(line)
		if !ok || label != "+CGPADDR" || len(fields) < 2 {
			continue
		}
		gotCID := intField(fields[0])
		if cid != 0 && gotCID != cid {
			continue
		}
		contexts = append(contexts, pdpContext{
			CID: gotCID,
			IP:  strings.Trim(fields[1], `"`),
			Raw: line,
		})
	}
	return contexts
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func parseCOPSOperators(raw []string) []networkOperator {
	operators := []networkOperator{}
	seen := map[string]bool{}
	for _, line := range raw {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "+COPS:") {
			continue
		}
		payload := strings.TrimSpace(strings.TrimPrefix(line, "+COPS:"))
		for _, group := range parenthesizedGroups(payload) {
			fields := parseCSVFields(group)
			if len(fields) < 4 {
				continue
			}
			status, err := strconv.Atoi(strings.TrimSpace(fields[0]))
			if err != nil {
				continue
			}
			act := -1
			if len(fields) > 4 {
				act = intField(fields[4])
			}
			op := networkOperator{
				Status:      status,
				StatusLabel: copsStatusLabel(status),
				LongName:    strings.Trim(fields[1], `"`),
				ShortName:   strings.Trim(fields[2], `"`),
				Numeric:     strings.Trim(fields[3], `"`),
				Act:         act,
				ActLabel:    actLabel(act),
				Raw:         group,
			}
			key := fmt.Sprintf("%s/%d/%d", op.Numeric, op.Act, op.Status)
			if op.Numeric == "" || seen[key] {
				continue
			}
			seen[key] = true
			operators = append(operators, op)
		}
	}
	return operators
}

func parenthesizedGroups(payload string) []string {
	groups := []string{}
	depth := 0
	start := -1
	inQuote := false
	for i, r := range payload {
		switch r {
		case '"':
			inQuote = !inQuote
		case '(':
			if !inQuote {
				if depth == 0 {
					start = i + 1
				}
				depth++
			}
		case ')':
			if !inQuote && depth > 0 {
				depth--
				if depth == 0 && start >= 0 && start <= i {
					groups = append(groups, payload[start:i])
				}
			}
		}
	}
	return groups
}

func parseCSVFields(value string) []string {
	reader := csv.NewReader(strings.NewReader(value))
	reader.TrimLeadingSpace = true
	reader.LazyQuotes = true
	reader.FieldsPerRecord = -1
	fields, err := reader.Read()
	if err != nil {
		return nil
	}
	for i := range fields {
		fields[i] = strings.TrimSpace(fields[i])
	}
	return fields
}

func copsStatusLabel(status int) string {
	switch status {
	case 0:
		return "未知"
	case 1:
		return "可用"
	case 2:
		return "当前"
	case 3:
		return "禁止"
	default:
		return fmt.Sprintf("状态%d", status)
	}
}

func ussdMessageFromURC(urc service.ModemURC) ussdMessage {
	message := ussdMessage{
		Status:  urc.Params[0],
		Message: urc.Params[1],
		DCS:     urc.Params[2],
	}
	message.Decoded = decodeUSSDText(message.Message, message.DCS)
	message.Raw = fmt.Sprintf("%s: %v", urc.Event, urc.Params)
	return message
}

func firstUSSDMessage(raw []string) (ussdMessage, bool) {
	for _, line := range raw {
		label, fields, ok := parseATLine(line)
		if !ok || label != "+CUSD" {
			continue
		}
		message := ussdMessage{Raw: line}
		if len(fields) > 0 {
			message.Status = strings.Trim(fields[0], `"`)
		}
		if len(fields) > 1 {
			message.Message = strings.Trim(fields[1], `"`)
		}
		if len(fields) > 2 {
			message.DCS = strings.Trim(fields[2], `"`)
		}
		message.Decoded = decodeUSSDText(message.Message, message.DCS)
		return message, true
	}
	return ussdMessage{}, false
}

func decodeUSSDText(value string, dcs string) string {
	text := strings.Trim(strings.TrimSpace(value), `"`)
	if text == "" {
		return ""
	}
	dcsCode := intField(dcs)
	if shouldDecodeUCS2(text, dcsCode) {
		if decoded := decodeUCS2Hex(text); decoded != "" {
			return decoded
		}
	}
	if isHexString(text) {
		if decoded := decodeASCIIHex(text); decoded != "" && utf8.ValidString(decoded) {
			return decoded
		}
	}
	return text
}

func shouldDecodeUCS2(value string, dcs int) bool {
	if dcs == 8 || dcs == 72 {
		return true
	}
	return isHexString(value) && len(cleanHex(value))%4 == 0 && strings.HasPrefix(strings.ToUpper(cleanHex(value)), "00")
}

func decodeUCS2Hex(value string) string {
	data, err := hex.DecodeString(cleanHex(value))
	if err != nil || len(data)%2 != 0 {
		return ""
	}
	words := make([]uint16, 0, len(data)/2)
	for i := 0; i+1 < len(data); i += 2 {
		words = append(words, uint16(data[i])<<8|uint16(data[i+1]))
	}
	return string(utf16.Decode(words))
}

func isHexString(value string) bool {
	cleaned := cleanHex(value)
	return cleaned != "" && len(cleaned) == len(strings.Trim(value, `"`))
}

func sanitizePhonebookStorage(storage string) string {
	storage = strings.ToUpper(strings.TrimSpace(storage))
	switch storage {
	case "SM", "ME", "MT", "ON":
		return storage
	default:
		return "SM"
	}
}

func parseCPBRRange(raw []string) (int, int, string) {
	for _, line := range raw {
		label, fields, ok := parseATLine(line)
		if !ok || label != "+CPBR" || len(fields) == 0 {
			continue
		}
		rangeText := strings.Trim(fields[0], "()")
		parts := strings.Split(rangeText, "-")
		if len(parts) != 2 {
			continue
		}
		start := intField(parts[0])
		end := intField(parts[1])
		return start, end, rangeText
	}
	return 0, 0, ""
}

func parseCPBREntries(raw []string) []phonebookEntry {
	entries := []phonebookEntry{}
	for _, line := range raw {
		label, fields, ok := parseATLine(line)
		if !ok || label != "+CPBR" || len(fields) < 4 {
			continue
		}
		name := strings.Trim(fields[3], `"`)
		if decoded := decodeMaybeHexText(name); decoded != "" {
			name = decoded
		}
		entries = append(entries, phonebookEntry{
			Index:  intField(fields[0]),
			Number: strings.Trim(fields[1], `"`),
			Type:   intField(fields[2]),
			Name:   name,
			Raw:    line,
		})
	}
	return entries
}

func decodeMaybeHexText(value string) string {
	cleaned := cleanHex(value)
	if cleaned == "" || len(cleaned) != len(value) {
		return ""
	}
	if len(cleaned)%4 == 0 {
		if decoded := decodeUCS2Hex(cleaned); decoded != "" {
			return decoded
		}
	}
	return decodeASCIIHex(cleaned)
}

func parseCSIMResponse(raw []string) H {
	result := H{}
	fields, ok := firstATFields(raw, "+CSIM")
	if !ok || len(fields) < 2 {
		return result
	}
	length := intField(fields[0])
	responseHex := strings.Trim(fields[1], `"`)
	result["length"] = length
	result["hex"] = responseHex
	if len(responseHex) >= 4 {
		result["sw1"] = responseHex[len(responseHex)-4 : len(responseHex)-2]
		result["sw2"] = responseHex[len(responseHex)-2:]
		result["data"] = responseHex[:len(responseHex)-4]
	}
	return result
}

func decodeICCID(hexValue string) string {
	data, err := hex.DecodeString(cleanHex(hexValue))
	if err != nil {
		return ""
	}
	var b strings.Builder
	for _, by := range data {
		lo := by & 0x0f
		hi := (by >> 4) & 0x0f
		if lo <= 9 {
			b.WriteByte('0' + lo)
		}
		if hi <= 9 && hi != 0x0f {
			b.WriteByte('0' + hi)
		}
	}
	return strings.TrimRight(b.String(), "Ff")
}

func decodeIMSI(hexValue string) string {
	cleaned := cleanHex(hexValue)
	if len(cleaned) < 4 {
		return ""
	}
	data, err := hex.DecodeString(cleaned)
	if err != nil || len(data) < 2 {
		return ""
	}
	var b strings.Builder
	for i, by := range data[1:] {
		lo := by & 0x0f
		hi := (by >> 4) & 0x0f
		if i == 0 {
			if hi <= 9 {
				b.WriteByte('0' + hi)
			}
			continue
		}
		if lo <= 9 {
			b.WriteByte('0' + lo)
		}
		if hi <= 9 && hi != 0x0f {
			b.WriteByte('0' + hi)
		}
	}
	return b.String()
}

func decodeASCIIHex(hexValue string) string {
	data, err := hex.DecodeString(cleanHex(hexValue))
	if err != nil {
		return ""
	}
	text := strings.TrimRight(string(data), "\x00\xff ")
	return strings.TrimSpace(text)
}

func decodeLooseBCD(hexValue string) string {
	data, err := hex.DecodeString(cleanHex(hexValue))
	if err != nil {
		return ""
	}
	var b strings.Builder
	for _, by := range data {
		for _, nibble := range []byte{by & 0x0f, (by >> 4) & 0x0f} {
			if nibble <= 9 {
				b.WriteByte('0' + nibble)
			} else if nibble == 0x0a {
				b.WriteByte('*')
			} else if nibble == 0x0b {
				b.WriteByte('#')
			}
		}
	}
	return strings.Trim(b.String(), "0")
}

func cleanHex(value string) string {
	value = strings.Trim(value, `"`)
	return strings.Map(func(r rune) rune {
		if (r >= '0' && r <= '9') || (r >= 'a' && r <= 'f') || (r >= 'A' && r <= 'F') {
			return r
		}
		return -1
	}, value)
}
