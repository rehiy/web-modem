package handler

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/rehiy/web-modem/service"
)

// ModemHandler 调制解调器处理器
type ModemHandler struct {
	ms *service.ModemService
}

// NewModemHandler 创建新的调制解调器处理器
func NewModemHandler() *ModemHandler {
	return &ModemHandler{
		ms: service.GetModemService(),
	}
}

// ListModems 返回可用调制解调器的列表
func (h *ModemHandler) ListModems(w http.ResponseWriter, r *http.Request) {
	h.ms.ScanModems()
	modems := h.ms.GetConnList()
	respondJSON(w, http.StatusOK, modems)
}

// SendModemCommand 向调制解调器发送原始 AT 命令
func (h *ModemHandler) SendModemCommand(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name    string `json:"name"`
		Command string `json:"command"`
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

	responses, err := conn.SendCommand(req.Command)
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

// GetModemBasicInfo 获取调制解调器基本信息
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

	info := H{"name": name}
	// 获取制造商
	if manufacturer, err := conn.GetManufacturer(); err == nil {
		info["manufacturer"] = manufacturer
	}
	// 获取型号
	if model, err := conn.GetModel(); err == nil {
		info["model"] = model
	}
	// 获取IMEI/序列号
	if imei, err := conn.GetIMEI(); err == nil {
		info["imei"] = imei
	}
	// 获取IMSI
	if imsi, err := conn.GetIMSI(); err == nil {
		info["imsi"] = imsi
	}
	// 获取手机号
	if number, _, err := conn.GetNumber(); err == nil {
		info["number"] = number
	}
	// 获取运营商（当前注册网络/Visited PLMN）
	if _, _, operator, act, err := conn.GetOperator(); err == nil {
		info["operator"] = operator
		info["act"] = act
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

	respondJSON(w, http.StatusOK, info)
}

// GetModemSignal 获取当前信号强度
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

	rssi, ber, err := conn.GetSignalQuality()
	if err != nil {
		respondJSON(w, http.StatusInternalServerError, H{"error": err.Error()})
		return
	}

	dbm := -113
	level := 0
	if rssi != 99 {
		dbm = (rssi * 2) - 113
		level = min(100, rssi*5)
	}
	respondJSON(w, http.StatusOK, map[string]any{
		"rssi":  rssi, // 通常是 0-31 的整数
		"ber":   ber,
		"level": level,
		"dbm":   dbm,
	})
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

	if err := conn.SendSmsPdu(req.Number, req.Message); err != nil {
		respondJSON(w, http.StatusInternalServerError, H{"error": err.Error()})
	} else {
		respondJSON(w, http.StatusOK, H{"status": "sent"})
	}
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
