package handler

import (
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/rehiy/modem/at"
	"github.com/rehiy/web-modem/database"
	"github.com/rehiy/web-modem/models"
	"github.com/rehiy/web-modem/service"
)

// ModemHandler 调制解调器处理器
type ModemHandler struct {
	ms *service.ModemService
}

var errNoATResponse = errors.New("no matching AT response")

// NewModemHandler 创建新的调制解调器处理器
func NewModemHandler() *ModemHandler {
	return &ModemHandler{
		ms: service.GetModemService(),
	}
}

// ListModems 返回在线设备列表。
//
// 默认直接返回后台监视维护的连接池快照（不访问串口，毫秒级响应）；
// 带 rescan=1 或服务刚启动尚未完成首次扫描时，现场扫描后再返回。
func (h *ModemHandler) ListModems(w http.ResponseWriter, r *http.Request) {
	if isTruthyQuery(r, "rescan") || !h.ms.HasScanned() {
		h.ms.RescanModems()
	}
	// 快照随设备状态变化，禁止浏览器/代理缓存
	setNoCache(w)
	respondJSON(w, http.StatusOK, h.ms.ListModemStates(true))
}

// isTruthyQuery 判断查询参数是否为真值（1/true/yes）。
func isTruthyQuery(r *http.Request, key string) bool {
	switch strings.ToLower(strings.TrimSpace(r.URL.Query().Get(key))) {
	case "1", "true", "yes":
		return true
	}
	return false
}

// SendModemCommand 向调制解调器发送原始 AT 命令
func (h *ModemHandler) SendModemCommand(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name    string `json:"name"`
		Command string `json:"command"`
		Timeout int    `json:"timeout"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondJSON(w, http.StatusBadRequest, H{"error": err.Error()})
		return
	}

	conn, err := h.ms.GetConn(req.Name)
	if conn == nil {
		respondJSON(w, http.StatusBadRequest, H{"error": err.Error()})
		return
	}
	// 原始 AT 指令可能修改任意状态，执行后使信息缓存失效
	defer h.ms.InvalidateModemCache(conn.Name)

	timeout := time.Duration(req.Timeout) * time.Second
	responses, err := conn.SendCommandWithTimeout(req.Command, timeout)
	if err != nil {
		respondJSON(w, http.StatusInternalServerError, H{"error": err.Error()})
		return
	}

	respondJSON(w, http.StatusOK, H{
		"name":     req.Name,
		"command":  req.Command,
		"response": strings.Join(responses, "\n"),
	})
}

// GetModemBasicInfo 获取调制解调器基本信息。
// 结果按设备缓存（见 service.LoadModemInfo），带 refresh=1 时强制现场查询。
func (h *ModemHandler) GetModemBasicInfo(w http.ResponseWriter, r *http.Request) {
	name := r.URL.Query().Get("name")
	if name == "" {
		respondJSON(w, http.StatusBadRequest, H{"error": "name is empty"})
		return
	}

	conn, err := h.ms.GetConn(name)
	if conn == nil {
		respondJSON(w, http.StatusBadRequest, H{"error": err.Error()})
		return
	}

	info, _ := h.ms.LoadModemInfo(conn.Name, isTruthyQuery(r, "refresh"), func() (map[string]any, error) {
		return queryModemInfo(conn), nil
	})
	setNoCache(w)
	respondJSON(w, http.StatusOK, info)
}

// queryModemInfo 现场查询设备基本信息，并同步更新连接上缓存的 SIM 身份字段。
func queryModemInfo(conn *service.ModemConn) H {
	state := conn.State()
	info := H{"name": conn.Name}
	if state.IMEI != "" {
		info["imei"] = state.IMEI
	}
	if state.Number != "" {
		info["number"] = state.Number
	}
	// 厂商/型号/固件/IMEI 连接时已读取，缺失时才现场查询
	if v := firstNonEmptyValue(state.Manufacturer, func() (string, error) { return conn.GetManufacturer() }); v != "" {
		info["manufacturer"] = v
	}
	if v := firstNonEmptyValue(state.Model, func() (string, error) { return conn.GetModel() }); v != "" {
		info["model"] = v
	}
	if v := firstNonEmptyValue(state.Revision, func() (string, error) { return conn.GetRevision() }); v != "" {
		info["revision"] = v
	}
	if state.IMEI == "" {
		if imei, err := conn.GetIMEI(); err == nil {
			info["imei"] = imei
			conn.SetIMEI(imei)
		}
	}
	// 获取 ICCID
	if iccid, err := getICCID(conn); err == nil {
		info["iccid"] = iccid
		conn.SetICCID(iccid)
	}
	// 获取IMSI
	if imsi, err := conn.GetIMSI(); err == nil {
		info["imsi"] = imsi
		conn.SetIMSI(imsi)
	}
	// 获取 SIM 状态
	if simStatus, err := conn.GetSIMStatus(); err == nil {
		info["sim_status"] = simStatus
		info["sim_status_label"] = simStatusLabel(simStatus)
	}
	// 获取手机号
	if number, _, err := conn.GetNumber(); err == nil {
		info["number"] = number
		conn.SetNumber(number)
	}
	// 获取运营商（当前注册网络/Visited PLMN）
	if _, _, operator, act, err := conn.GetOperator(); err == nil {
		info["operator"] = operator
		conn.SetOperator(operator)
		info["act"] = act
		info["act_label"] = actLabel(act)
	}
	if mode, err := conn.GetNetworkMode(); err == nil {
		info["network_mode"] = mode
		info["network_mode_label"] = networkModeLabel(mode)
	}
	if _, stat, err := conn.GetNetworkStatus(); err == nil {
		addRegistrationInfo(info, "cs", stat)
	}
	if _, stat, err := conn.GetGPRSStatus(); err == nil {
		addRegistrationInfo(info, "packet", stat)
	}
	if _, stat, err := queryRegistration(conn, "AT+CEREG?", "+CEREG"); err == nil {
		addRegistrationInfo(info, "eps", stat)
		info["registration_status_code"] = stat
		info["registration_status"] = registrationStatusLabel(stat)
		info["roaming"] = stat == 5
		info["roaming_label"] = roamingLabel(stat == 5)
	}
	// 获取短信中心
	if center, _, err := conn.GetSmsCenter(); err == nil {
		info["sms_center"] = center
	}
	// 获取短信模式
	if mode, err := conn.GetSmsMode(); err == nil {
		info["sms_mode"] = "text"
		if mode == 0 {
			info["sms_mode"] = "pdu"
		}
	}
	return info
}

// firstNonEmptyValue 优先返回已缓存的值，为空时调用 query 现场读取。
func firstNonEmptyValue(cached string, query func() (string, error)) string {
	if cached != "" {
		return cached
	}
	if v, err := query(); err == nil {
		return strings.TrimSpace(v)
	}
	return ""
}

// GetModemSignal 获取当前信号强度。
// 结果按设备短时缓存，带 refresh=1 时强制现场查询。
func (h *ModemHandler) GetModemSignal(w http.ResponseWriter, r *http.Request) {
	name := r.URL.Query().Get("name")
	if name == "" {
		respondJSON(w, http.StatusBadRequest, H{"error": "name is empty"})
		return
	}

	conn, err := h.ms.GetConn(name)
	if conn == nil {
		respondJSON(w, http.StatusBadRequest, H{"error": err.Error()})
		return
	}

	signal, err := h.ms.LoadModemSignal(conn.Name, isTruthyQuery(r, "refresh"), func() (map[string]any, error) {
		return querySignal(conn)
	})
	if err != nil {
		respondJSON(w, http.StatusInternalServerError, H{"error": err.Error()})
		return
	}
	setNoCache(w)
	respondJSON(w, http.StatusOK, signal)
}

// querySignal 现场查询信号质量及扩展的小区/射频信息。
func querySignal(conn *service.ModemConn) (map[string]any, error) {
	rssi, ber, err := conn.GetSignalQuality()
	if err != nil {
		return nil, err
	}

	dbm := -113
	level := 0
	if rssi != 99 {
		dbm = (rssi * 2) - 113
		level = min(100, rssi*5)
	}
	signal := map[string]any{
		"rssi":      rssi, // 通常是 0-31 的整数
		"ber":       ber,
		"ber_label": berLabel(ber),
		"level":     level,
		"dbm":       dbm,
	}
	addExtendedSignal(conn, signal)
	return signal, nil
}

func getICCID(conn *service.ModemConn) (string, error) {
	if iccid, err := conn.GetICCID(); err == nil {
		if normalized := normalizeICCID(iccid); normalized != "" {
			return normalized, nil
		}
	}

	for _, command := range []string{"AT+QCCID", "AT+ICCID?", "AT+CCID?"} {
		responses, err := conn.SendOptionalCommand(command)
		if err != nil {
			continue
		}
		if fields, ok := firstATFields(responses, "+QCCID", "+ICCID", "+CCID"); ok && len(fields) > 0 {
			if iccid := normalizeICCID(fields[0]); iccid != "" {
				return iccid, nil
			}
		}
		for _, line := range responses {
			if iccid := normalizeICCID(line); iccid != "" {
				return iccid, nil
			}
		}
	}

	return "", errNoATResponse
}

func queryRegistration(conn *service.ModemConn, command string, label string) (int, int, error) {
	responses, err := conn.SendOptionalCommand(command)
	if err != nil {
		return 0, 0, err
	}

	fields, ok := firstATFields(responses, label)
	if !ok {
		return 0, 0, errNoATResponse
	}
	if len(fields) >= 2 {
		return intField(fields[0]), intField(fields[1]), nil
	}
	if len(fields) == 1 {
		return 0, intField(fields[0]), nil
	}
	return 0, 0, errNoATResponse
}

func addRegistrationInfo(info H, prefix string, stat int) {
	codeKey := prefix + "_registration_status_code"
	labelKey := prefix + "_registration_status"
	info[codeKey] = stat
	info[labelKey] = registrationStatusLabel(stat)

	if _, ok := info["registration_status_code"]; !ok {
		info["registration_status_code"] = stat
		info["registration_status"] = registrationStatusLabel(stat)
		info["roaming"] = stat == 5
		info["roaming_label"] = roamingLabel(stat == 5)
	}
}

func addExtendedSignal(conn *service.ModemConn, signal map[string]any) {
	if values, err := queryCESQ(conn); err == nil {
		for key, value := range values {
			signal[key] = value
		}
	}
	if values, err := queryQCSQ(conn); err == nil {
		for key, value := range values {
			if _, exists := signal[key]; !exists || key == "radio" || key == "diagnostic_source" {
				signal[key] = value
			}
		}
	}
	if values, err := queryQENGServingCell(conn); err == nil {
		mergeSignalValues(signal, values, false)
	}
	if values, err := queryRegistrationCell(conn); err == nil {
		mergeSignalValues(signal, values, false)
	}
	if !hasCellIdentity(signal) {
		if values, err := queryMUESTATSRadio(conn); err == nil {
			mergeSignalValues(signal, values, false)
		}
	}
}

func mergeSignalValues(signal map[string]any, values map[string]any, overwrite bool) {
	for key, value := range values {
		if overwrite {
			signal[key] = value
			continue
		}
		if _, exists := signal[key]; !exists {
			signal[key] = value
		}
	}
}

func hasCellIdentity(signal map[string]any) bool {
	for _, key := range []string{"cell_id", "tac", "lac", "pci", "earfcn", "arfcn"} {
		if value, ok := signal[key]; ok && strings.TrimSpace(fmt.Sprint(value)) != "" {
			return true
		}
	}
	return false
}

func queryCESQ(conn *service.ModemConn) (map[string]any, error) {
	responses, err := conn.SendOptionalCommand("AT+CESQ")
	if err != nil {
		return nil, err
	}

	fields, ok := firstATFields(responses, "+CESQ")
	if !ok || len(fields) < 6 {
		return nil, errNoATResponse
	}

	result := map[string]any{
		"diagnostic_source": "CESQ",
	}

	rscp := intField(fields[2])
	ecn0 := intField(fields[3])
	rsrq := intField(fields[4])
	rsrp := intField(fields[5])
	if rscp >= 0 && rscp <= 96 {
		result["rscp"] = -120 + rscp
	}
	if ecn0 >= 0 && ecn0 <= 49 {
		result["ecn0"] = float64(ecn0)/2 - 24.5
	}
	if rsrq >= 0 && rsrq <= 34 {
		result["rsrq"] = float64(rsrq)/2 - 19.5
	}
	if rsrp >= 0 && rsrp <= 97 {
		result["rsrp"] = -140 + rsrp
	}

	return result, nil
}

func queryQCSQ(conn *service.ModemConn) (map[string]any, error) {
	responses, err := conn.SendOptionalCommand("AT+QCSQ")
	if err != nil {
		return nil, err
	}

	fields, ok := firstATFields(responses, "+QCSQ")
	if !ok || len(fields) < 2 {
		return nil, errNoATResponse
	}

	result := map[string]any{
		"radio":             fields[0],
		"diagnostic_source": "QCSQ",
	}
	switch strings.ToUpper(fields[0]) {
	case "LTE", "CAT-M1", "CAT-NB1", "NBIOT":
		if len(fields) >= 5 {
			result["rssi_dbm"] = intField(fields[1])
			result["rsrp"] = intField(fields[2])
			result["sinr"] = intField(fields[3])
			result["rsrq"] = intField(fields[4])
		}
	case "GSM":
		result["rssi_dbm"] = intField(fields[1])
	default:
		for i := 1; i < len(fields); i++ {
			result["qcsq_"+strconv.Itoa(i)] = fields[i]
		}
	}

	return result, nil
}

func queryRegistrationCell(conn *service.ModemConn) (map[string]any, error) {
	queries := []struct {
		command string
		label   string
		source  string
	}{
		{"AT+CEREG?", "+CEREG", "CEREG"},
		{"AT+CGREG?", "+CGREG", "CGREG"},
		{"AT+CREG?", "+CREG", "CREG"},
	}

	for _, query := range queries {
		responses, err := conn.SendOptionalCommand(query.command)
		if err != nil {
			continue
		}
		fields, ok := firstATFields(responses, query.label)
		if !ok || len(fields) < 4 {
			continue
		}

		result := map[string]any{
			"cell_source": query.source,
			"cell_raw":    firstMatchingLine(responses, query.label),
		}
		if query.label == "+CEREG" {
			result["tac"] = fields[2]
		} else {
			result["lac"] = fields[2]
		}
		result["cell_id"] = fields[3]
		if len(fields) > 4 {
			result["act"] = intField(fields[4])
			result["act_label"] = actLabel(intField(fields[4]))
		}
		return result, nil
	}

	return nil, errNoATResponse
}

func queryQENGServingCell(conn *service.ModemConn) (map[string]any, error) {
	responses, err := conn.SendOptionalCommand(`AT+QENG="servingcell"`)
	if err != nil {
		return nil, err
	}

	result := map[string]any{
		"cell_source": "QENG",
		"cell_raw":    compactATPayload(responses, "+QENG"),
	}
	for _, line := range responses {
		label, fields, ok := parseATLine(line)
		if !ok || label != "+QENG" {
			continue
		}
		mergeSignalValues(result, parseQENGFields(fields), false)
	}

	if len(result) <= 2 {
		return nil, errNoATResponse
	}
	return result, nil
}

func parseQENGFields(fields []string) map[string]any {
	result := map[string]any{}
	ratIdx := indexOfAnyField(fields, "LTE", "WCDMA", "GSM", "NR5G-NSA", "NR5G-SA")
	if ratIdx < 0 {
		if len(fields) > 1 && strings.EqualFold(fields[0], "servingcell") {
			result["cell_state"] = fields[1]
		}
		return result
	}

	rat := strings.ToUpper(fields[ratIdx])
	result["radio"] = rat
	if ratIdx > 0 && !strings.EqualFold(fields[ratIdx-1], "servingcell") {
		result["cell_state"] = fields[ratIdx-1]
	}

	values := fields[ratIdx+1:]
	switch rat {
	case "LTE":
		parseQENGLTE(result, values)
	case "WCDMA":
		parseQENGWCDMA(result, values)
	case "GSM":
		parseQENGGSM(result, values)
	case "NR5G-NSA", "NR5G-SA":
		parseQENGNR(result, values)
	}
	return result
}

func parseQENGLTE(result map[string]any, values []string) {
	if len(values) < 15 {
		return
	}
	result["duplex"] = values[0]
	result["mcc"] = values[1]
	result["mnc"] = values[2]
	result["cell_id"] = values[3]
	result["pci"] = values[4]
	result["earfcn"] = values[5]
	result["band"] = values[6]
	result["tac"] = values[10]
	result["rsrp"] = intField(values[11])
	result["rsrq"] = intField(values[12])
	result["rssi_dbm"] = intField(values[13])
	result["sinr"] = intField(values[14])
}

func parseQENGWCDMA(result map[string]any, values []string) {
	if len(values) < 9 {
		return
	}
	result["mcc"] = values[0]
	result["mnc"] = values[1]
	result["lac"] = values[2]
	result["cell_id"] = values[3]
	result["uarfcn"] = values[4]
	result["psc"] = values[5]
	result["rac"] = values[6]
	result["rscp"] = intField(values[7])
	result["ecn0"] = intField(values[8])
}

func parseQENGGSM(result map[string]any, values []string) {
	if len(values) < 7 {
		return
	}
	result["mcc"] = values[0]
	result["mnc"] = values[1]
	result["lac"] = values[2]
	result["cell_id"] = values[3]
	result["bsic"] = values[4]
	result["arfcn"] = values[5]
	result["band"] = values[6]
	if len(values) > 7 {
		result["rxlev"] = values[7]
	}
}

func parseQENGNR(result map[string]any, values []string) {
	if len(values) < 6 {
		return
	}
	result["nr_mcc"] = values[0]
	result["nr_mnc"] = values[1]
	result["nr_pci"] = values[2]
	result["nr_rsrp"] = intField(values[3])
	result["nr_sinr"] = intField(values[4])
	result["nr_rsrq"] = intField(values[5])
}

func queryMUESTATSRadio(conn *service.ModemConn) (map[string]any, error) {
	responses, err := conn.SendOptionalCommand(`AT+MUESTATS="radio"`)
	if err != nil {
		return nil, err
	}

	fields, ok := firstATFields(responses, "+MUESTATS")
	if !ok || len(fields) < 2 || !strings.EqualFold(fields[0], "radio") {
		return nil, errNoATResponse
	}

	result := map[string]any{
		"cell_source": "MUESTATS radio",
		"cell_raw":    strings.Join(fields[1:], ", "),
	}
	parseKeyValueCellFields(result, fields[1:])
	return result, nil
}

func parseKeyValueCellFields(result map[string]any, fields []string) {
	for _, field := range fields {
		key, value, ok := splitLooseKeyValue(field)
		if !ok {
			continue
		}
		switch normalizeATKey(key) {
		case "mcc":
			result["mcc"] = value
		case "mnc":
			result["mnc"] = value
		case "tac":
			result["tac"] = value
		case "lac":
			result["lac"] = value
		case "ci", "cellid", "cell_id", "cell":
			result["cell_id"] = value
		case "pci", "pcid":
			result["pci"] = value
		case "earfcn", "earfcn_dl":
			result["earfcn"] = value
		case "arfcn":
			result["arfcn"] = value
		case "band":
			result["band"] = value
		case "rsrp":
			result["rsrp"] = intField(value)
		case "rsrq":
			result["rsrq"] = intField(value)
		case "sinr":
			result["sinr"] = intField(value)
		case "rssi":
			result["rssi_dbm"] = intField(value)
		case "rat", "radio", "mode":
			result["radio"] = value
		}
	}
}

func splitLooseKeyValue(field string) (string, string, bool) {
	for _, sep := range []string{"=", ":"} {
		key, value, ok := strings.Cut(field, sep)
		if ok {
			return strings.TrimSpace(key), strings.Trim(strings.TrimSpace(value), `"'`), true
		}
	}
	parts := strings.Fields(field)
	if len(parts) == 2 {
		return parts[0], strings.Trim(parts[1], `"'`), true
	}
	return "", "", false
}

func normalizeATKey(key string) string {
	key = strings.ToLower(strings.TrimSpace(key))
	key = strings.ReplaceAll(key, "-", "_")
	key = strings.ReplaceAll(key, " ", "_")
	return key
}

func indexOfAnyField(fields []string, values ...string) int {
	for i, field := range fields {
		for _, value := range values {
			if strings.EqualFold(field, value) {
				return i
			}
		}
	}
	return -1
}

func firstMatchingLine(responses []string, label string) string {
	for _, line := range responses {
		if strings.HasPrefix(strings.TrimSpace(line), label+":") {
			return strings.TrimSpace(line)
		}
	}
	return ""
}

func compactATPayload(responses []string, label string) string {
	lines := []string{}
	for _, line := range responses {
		if strings.HasPrefix(strings.TrimSpace(line), label+":") {
			lines = append(lines, strings.TrimSpace(line))
		}
	}
	return strings.Join(lines, "\n")
}

func firstATFields(responses []string, labels ...string) ([]string, bool) {
	for _, line := range responses {
		label, fields, ok := parseATLine(line)
		if !ok {
			continue
		}
		for _, expected := range labels {
			if label == expected {
				return fields, true
			}
		}
	}
	return nil, false
}

func parseATLine(line string) (string, []string, bool) {
	label, rest, ok := strings.Cut(strings.TrimSpace(line), ":")
	if !ok {
		return "", nil, false
	}

	reader := csv.NewReader(strings.NewReader(rest))
	reader.TrimLeadingSpace = true
	reader.LazyQuotes = true
	reader.FieldsPerRecord = -1
	fields, err := reader.Read()
	if err != nil {
		return "", nil, false
	}
	for i, field := range fields {
		fields[i] = strings.TrimSpace(field)
	}
	return strings.TrimSpace(label), fields, true
}

func normalizeDigits(value string) string {
	return strings.Map(func(r rune) rune {
		if r >= '0' && r <= '9' {
			return r
		}
		return -1
	}, value)
}

func normalizeICCID(value string) string {
	iccid := normalizeDigits(value)
	if len(iccid) < 18 || len(iccid) > 22 {
		return ""
	}
	return iccid
}

func intField(value string) int {
	n, _ := strconv.Atoi(strings.TrimSpace(value))
	return n
}

func simStatusLabel(status string) string {
	switch strings.ToUpper(strings.TrimSpace(status)) {
	case "READY":
		return "就绪"
	case "SIM PIN":
		return "需要 PIN"
	case "SIM PUK":
		return "需要 PUK"
	case "PH-SIM PIN":
		return "需要设备 PIN"
	case "ERROR":
		return "读取失败"
	default:
		if strings.TrimSpace(status) == "" {
			return "-"
		}
		return status
	}
}

func registrationStatusLabel(stat int) string {
	switch stat {
	case 0:
		return "未注册"
	case 1:
		return "已注册"
	case 2:
		return "搜索中"
	case 3:
		return "注册被拒绝"
	case 4:
		return "未知"
	case 5:
		return "漫游注册"
	default:
		return "未知"
	}
}

func roamingLabel(roaming bool) string {
	if roaming {
		return "是"
	}
	return "否"
}

func actLabel(act int) string {
	switch act {
	case 0:
		return "GSM"
	case 2:
		return "UTRAN"
	case 3:
		return "GSM/EGPRS"
	case 4:
		return "UTRAN/HSDPA"
	case 5:
		return "E-UTRA"
	case 6:
		return "E-UTRA NB"
	case 7:
		return "LTE"
	default:
		return "-"
	}
}

func networkModeLabel(mode int) string {
	switch mode {
	case 2:
		return "自动"
	case 13:
		return "GSM ONLY"
	case 38:
		return "LTE ONLY"
	case 51:
		return "SA/NSA"
	default:
		return strconv.Itoa(mode)
	}
}

func berLabel(ber int) string {
	switch {
	case ber >= 0 && ber <= 7:
		return strconv.Itoa(ber)
	case ber == 99:
		return "未知"
	default:
		return "-"
	}
}

// SendModemSms 发送短信
func (h *ModemHandler) SendModemSms(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name    string `json:"name"`
		Number  string `json:"number"`
		Message string `json:"message"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondJSON(w, http.StatusBadRequest, H{"error": err.Error()})
		return
	}

	conn, err := h.ms.GetConn(req.Name)
	if conn == nil {
		respondJSON(w, http.StatusBadRequest, H{"error": err.Error()})
		return
	}

	sms := &models.Sms{
		Content:       req.Message,
		SmsIDs:        "",
		ReceiveTime:   time.Now(),
		ReceiveNumber: req.Number,
		SendNumber:    conn.State().Number,
		Direction:     "out",
		ModemName:     conn.Name,
		DeviceIMEI:    conn.State().IMEI,
	}
	service.EnrichSmsCardInfo(sms, conn)
	webhookService := service.NewWebhookService()

	if err := conn.SendSmsPdu(req.Number, req.Message); err != nil {
		if alertErr := service.RaiseAlert("sms_send_failed", "warning", conn.Name, "短信发送失败", err.Error()); alertErr != nil {
			log.Printf("[%s] Failed to raise sms send alert: %v", req.Name, alertErr)
		}
		webhookService.HandleSmsSentFailed(sms, err)
		respondJSON(w, http.StatusInternalServerError, H{"error": err.Error()})
		return
	}
	service.ResolveAlert("sms_send_failed", conn.Name)

	if database.IsSmsdbEnabled() {
		if err := database.CreateSms(sms); err != nil {
			log.Printf("[%s] Failed to save outgoing Sms: %v", req.Name, err)
		}
	}
	webhookService.HandleSmsSentSuccess(sms)

	respondJSON(w, http.StatusOK, H{"status": "sent"})
}

// ListModemSms 获取调制解调器中的所有短信
func (h *ModemHandler) ListModemSms(w http.ResponseWriter, r *http.Request) {
	name := r.URL.Query().Get("name")
	if name == "" {
		respondJSON(w, http.StatusBadRequest, H{"error": "name is empty"})
		return
	}

	conn, err := h.ms.GetConn(name)
	if conn == nil {
		respondJSON(w, http.StatusBadRequest, H{"error": err.Error()})
		return
	}

	smsList, err := conn.ListSmsPdu(4)
	if err != nil {
		respondJSON(w, http.StatusInternalServerError, H{"error": err.Error()})
		return
	}
	if smsList == nil {
		smsList = []at.Sms{}
	}

	respondJSON(w, http.StatusOK, smsList)
}

// DeleteModemSms 删除短信
func (h *ModemHandler) DeleteModemSms(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name    string `json:"name"`
		Indices []int  `json:"indices"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondJSON(w, http.StatusBadRequest, H{"error": err.Error()})
		return
	}

	conn, err := h.ms.GetConn(req.Name)
	if conn == nil {
		respondJSON(w, http.StatusBadRequest, H{"error": err.Error()})
		return
	}

	if err := conn.DeleteSms(req.Indices); err != nil {
		respondJSON(w, http.StatusInternalServerError, H{"error": err.Error()})
	} else {
		respondJSON(w, http.StatusOK, H{"status": "deleted", "count": len(req.Indices)})
	}
}
