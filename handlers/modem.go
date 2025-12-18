package handlers

import (
	"encoding/json"
	"net/http"

	"modem-manager/modem"
	"modem-manager/services"
)

var serialManager = services.GetSerialManager()

// ListModems 返回可用调制解调器的列表
func ListModems(w http.ResponseWriter, r *http.Request) {
	if ports, err := serialManager.Scan(115200); err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
	} else {
		respondJSON(w, http.StatusOK, ports)
	}
}

// SendATCommand 向调制解调器发送原始 AT 命令
func SendATCommand(w http.ResponseWriter, r *http.Request) {
	var cmd modem.ATCommand
	if err := json.NewDecoder(r.Body).Decode(&cmd); err != nil {
		respondError(w, http.StatusBadRequest, "Invalid request")
		return
	}

	if svc := getService(w, cmd.Port); svc != nil {
		var err error
		if cmd.Response, err = svc.SendATCommand(cmd.Command); err != nil {
			cmd.Error = err.Error()
		}
		respondJSON(w, http.StatusOK, cmd)
	}
}

// GetModemInfo 获取有关调制解调器的详细信息
func GetModemInfo(w http.ResponseWriter, r *http.Request) {
	if svc := getService(w, r.URL.Query().Get("port")); svc != nil {
		if info, err := svc.GetModemInfo(); err != nil {
			respondError(w, http.StatusInternalServerError, err.Error())
		} else {
			respondJSON(w, http.StatusOK, info)
		}
	}
}

// GetSignalStrength 获取当前信号强度
func GetSignalStrength(w http.ResponseWriter, r *http.Request) {
	if svc := getService(w, r.URL.Query().Get("port")); svc != nil {
		if signal, err := svc.GetSignalStrength(); err != nil {
			respondError(w, http.StatusInternalServerError, err.Error())
		} else {
			respondJSON(w, http.StatusOK, signal)
		}
	}
}

// ListSMS 获取调制解调器中的所有短信
func ListSMS(w http.ResponseWriter, r *http.Request) {
	if svc := getService(w, r.URL.Query().Get("port")); svc != nil {
		if list, err := svc.ListSMS(); err != nil {
			respondError(w, http.StatusInternalServerError, err.Error())
		} else {
			respondJSON(w, http.StatusOK, list)
		}
	}
}

// SendSMS 发送短信
func SendSMS(w http.ResponseWriter, r *http.Request) {
	var req modem.SendSMSRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "Invalid request")
		return
	}

	if svc := getService(w, req.Port); svc != nil {
		if err := svc.SendSMS(req.Number, req.Message); err != nil {
			respondError(w, http.StatusInternalServerError, err.Error())
		} else {
			respondJSON(w, http.StatusOK, map[string]string{"status": "sent"})
		}
	}
}

// 辅助函数

func respondJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

func respondError(w http.ResponseWriter, status int, msg string) {
	respondJSON(w, status, map[string]string{"error": msg})
}

func getService(w http.ResponseWriter, port string) *modem.SerialService {
	if port == "" {
		respondError(w, http.StatusBadRequest, "port is required")
		return nil
	}
	svc, err := serialManager.GetService(port)
	if err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return nil
	}
	return svc
}
