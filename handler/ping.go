package handler

import (
	"encoding/json"
	"net/http"

	"github.com/rehiy/web-modem/service"
)

// PingHandler 连通性测试处理器
type PingHandler struct {
	ms *service.ModemService
}

// NewPingHandler 创建连通性测试处理器
func NewPingHandler() *PingHandler {
	return &PingHandler{ms: service.GetModemService()}
}

type pingRequest struct {
	Name  string `json:"name"`  // 模组端口名（模组 ping 必填）
	Host  string `json:"host"`  // 测试目标，留空默认 8.8.8.8
	Count int    `json:"count"` // 包数，默认 4，上限 10
}

// ModemPing 通过模组厂商 AT 指令执行 ping，检测蜂窝数据链路连通性
func (h *PingHandler) ModemPing(w http.ResponseWriter, r *http.Request) {
	var req pingRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondJSON(w, http.StatusBadRequest, H{"error": err.Error()})
		return
	}
	if req.Name == "" {
		respondJSON(w, http.StatusBadRequest, H{"error": "name is empty"})
		return
	}

	result, err := h.ms.PingFromModem(req.Name, req.Host, req.Count)
	if err != nil {
		respondJSON(w, http.StatusInternalServerError, H{"ok": false, "error": err.Error()})
		return
	}
	respondJSON(w, http.StatusOK, H{"ok": true, "data": result})
}

// HostPing 在运行本服务的主机上执行系统 ping，检测主机网络连通性
func (h *PingHandler) HostPing(w http.ResponseWriter, r *http.Request) {
	var req pingRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondJSON(w, http.StatusBadRequest, H{"error": err.Error()})
		return
	}

	result, err := service.PingFromHost(req.Host, req.Count)
	if err != nil {
		respondJSON(w, http.StatusInternalServerError, H{"ok": false, "error": err.Error()})
		return
	}
	respondJSON(w, http.StatusOK, H{"ok": true, "data": result})
}
