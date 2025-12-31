package handlers

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/rehiy/web-modem/services"
)

type H map[string]any

var serialManager = services.GetModemManager()

// ListModems 返回可用调制解调器的列表
func ListModems(w http.ResponseWriter, r *http.Request) {
	serialManager.ScanModems()
	modems := serialManager.GetModems()
	respondJSON(w, http.StatusOK, modems)
}

// SendATCommand 向调制解调器发送原始 AT 命令
func SendATCommand(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name    string `json:"name"`
		Command string `json:"command"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondJSON(w, http.StatusBadRequest, H{"error": err.Error()})
		return
	}

	conn, err := serialManager.GetConnect(req.Name)
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
func GetModemInfo(w http.ResponseWriter, r *http.Request) {
	name := r.URL.Query().Get("name")
	if name == "" {
		respondJSON(w, http.StatusBadRequest, H{"error": "name is empty"})
		return
	}

	conn, err := serialManager.GetConnect(name)
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
func GetSignalStrength(w http.ResponseWriter, r *http.Request) {
	name := r.URL.Query().Get("name")
	if name == "" {
		respondJSON(w, http.StatusBadRequest, H{"error": "name is empty"})
		return
	}

	conn, err := serialManager.GetConnect(name)
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

// ListSMS 获取调制解调器中的所有短信
func ListSMS(w http.ResponseWriter, r *http.Request) {
	name := r.URL.Query().Get("name")
	if name == "" {
		respondJSON(w, http.StatusBadRequest, H{"error": "name is empty"})
		return
	}

	conn, err := serialManager.GetConnect(name)
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

// SendSMS 发送短信
func SendSMS(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name    string `json:"name"`
		Number  string `json:"number"`
		Message string `json:"message"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondJSON(w, http.StatusBadRequest, H{"error": err.Error()})
		return
	}

	conn, err := serialManager.GetConnect(req.Name)
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

// DeleteSMS 删除短信
func DeleteSMS(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name  string `json:"name"`
		Index int    `json:"index"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondJSON(w, http.StatusBadRequest, H{"error": err.Error()})
		return
	}

	conn, err := serialManager.GetConnect(req.Name)
	if conn == nil {
		respondJSON(w, http.StatusBadRequest, H{"error": err.Error()})
		return
	}

	if err := conn.DeleteSMS(req.Index); err != nil {
		respondJSON(w, http.StatusInternalServerError, H{"error": err.Error()})
	} else {
		respondJSON(w, http.StatusOK, H{"status": "deleted"})
	}
}

// 辅助函数

func respondJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}
