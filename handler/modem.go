package handler

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/gorilla/websocket"
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
	modems := h.ms.GetModems()
	respondJSON(w, http.StatusOK, modems)
}

// SendATCommand 向调制解调器发送原始 AT 命令
func (h *ModemHandler) SendATCommand(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name    string `json:"name"`
		Command string `json:"command"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondJSON(w, http.StatusBadRequest, H{"error": err.Error()})
		return
	}

	conn, err := h.ms.GetConnect(req.Name)
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

// GetModemInfo 获取有关调制解调器的详细信息
func (h *ModemHandler) GetModemInfo(w http.ResponseWriter, r *http.Request) {
	name := r.URL.Query().Get("name")
	if name == "" {
		respondJSON(w, http.StatusBadRequest, H{"error": "name is empty"})
		return
	}

	conn, err := h.ms.GetConnect(name)
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
	if imei, err := conn.GetSerialNumber(); err == nil {
		info["imei"] = imei
	}
	// 获取IMSI
	if imsi, err := conn.GetIMSI(); err == nil {
		info["imsi"] = imsi
	}
	// 获取运营商
	if _, _, operator, act, err := conn.GetOperator(); err == nil {
		info["operator"] = operator
		info["act"] = act
	}
	// 获取手机号
	if phone, _, err := conn.GetPhoneNumber(); err == nil {
		info["phone"] = phone
	}

	respondJSON(w, http.StatusOK, info)
}

// GetSignalStrength 获取当前信号强度
func (h *ModemHandler) GetSignalStrength(w http.ResponseWriter, r *http.Request) {
	name := r.URL.Query().Get("name")
	if name == "" {
		respondJSON(w, http.StatusBadRequest, H{"error": "name is empty"})
		return
	}

	conn, err := h.ms.GetConnect(name)
	if conn == nil {
		respondJSON(w, http.StatusBadRequest, H{"error": err.Error()})
		return
	}

	rssi, ber, err := conn.GetSignalQuality()
	if err != nil {
		respondJSON(w, http.StatusInternalServerError, H{"error": err.Error()})
		return
	}

	// 计算信号等级
	level := 0
	if rssi >= 20 {
		level = 5
	} else if rssi >= 15 {
		level = 4
	} else if rssi >= 10 {
		level = 3
	} else if rssi >= 5 {
		level = 2
	} else if rssi >= 1 {
		level = 1
	}

	// 将 RSSI 转换为 dBm: dBm = -113 + (rssi * 2)
	dbm := -113 + (rssi * 2)

	respondJSON(w, http.StatusOK, map[string]any{
		"rssi":  rssi,
		"ber":   ber,
		"level": level,
		"dbm":   dbm,
	})
}

// SendSMS 发送短信
func (h *ModemHandler) SendSMS(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name    string `json:"name"`
		Number  string `json:"number"`
		Message string `json:"message"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondJSON(w, http.StatusBadRequest, H{"error": err.Error()})
		return
	}

	conn, err := h.ms.GetConnect(req.Name)
	if conn == nil {
		respondJSON(w, http.StatusBadRequest, H{"error": err.Error()})
		return
	}

	if err := conn.SendSMSPdu(req.Number, req.Message); err != nil {
		respondJSON(w, http.StatusInternalServerError, H{"error": err.Error()})
	} else {
		respondJSON(w, http.StatusOK, H{"status": "sent"})
	}
}

// ListSMS 获取调制解调器中的所有短信
func (h *ModemHandler) ListSMS(w http.ResponseWriter, r *http.Request) {
	name := r.URL.Query().Get("name")
	if name == "" {
		respondJSON(w, http.StatusBadRequest, H{"error": "name is empty"})
		return
	}

	conn, err := h.ms.GetConnect(name)
	if conn == nil {
		respondJSON(w, http.StatusBadRequest, H{"error": err.Error()})
		return
	}

	smsList, err := conn.ListSMSPdu(4)
	if err != nil {
		respondJSON(w, http.StatusInternalServerError, H{"error": err.Error()})
		return
	}

	respondJSON(w, http.StatusOK, smsList)
}

// DeleteSMS 删除短信
func (h *ModemHandler) DeleteSMS(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name    string `json:"name"`
		Indices []int  `json:"indices"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondJSON(w, http.StatusBadRequest, H{"error": err.Error()})
		return
	}

	conn, err := h.ms.GetConnect(req.Name)
	if conn == nil {
		respondJSON(w, http.StatusBadRequest, H{"error": err.Error()})
		return
	}

	if err := conn.DeleteSMS(req.Indices); err != nil {
		respondJSON(w, http.StatusInternalServerError, H{"error": err.Error()})
	} else {
		respondJSON(w, http.StatusOK, H{"status": "deleted", "count": len(req.Indices)})
	}
}

// HandleWebSocket 将 HTTP 连接升级为 WebSocket 连接
// 并将串口事件流式传输到客户端
func (h *ModemHandler) HandleWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer conn.Close()

	// 读取消息并推送到客户端
	for msg := range service.ModemEvent {
		if err := conn.WriteMessage(websocket.TextMessage, []byte(msg)); err != nil {
			return
		}
	}
}
