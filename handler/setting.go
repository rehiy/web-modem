package handler

import (
	"encoding/json"
	"net/http"

	"github.com/rehiy/web-modem/database"
)

// SettingHandler 设置处理器
type SettingHandler struct{}

// NewSettingHandler 创建新的设置处理器
func NewSettingHandler() *SettingHandler {
	return &SettingHandler{}
}

// GetSettings 获取所有设置
func (h *SettingHandler) GetSettings(w http.ResponseWriter, r *http.Request) {
	settings, err := database.GetSettings()
	if err != nil {
		respondJSON(w, http.StatusInternalServerError, H{"error": err.Error()})
		return
	}

	respondJSON(w, http.StatusOK, settings)
}

// UpdateSmsdbSettings 更新短信存储设置
func (h *SettingHandler) UpdateSmsdbSettings(w http.ResponseWriter, r *http.Request) {
	var req struct {
		SmsdbEnabled bool `json:"smsdb_enabled"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondJSON(w, http.StatusBadRequest, H{"error": err.Error()})
		return
	}

	if err := database.SetSmsdbEnabled(req.SmsdbEnabled); err != nil {
		respondJSON(w, http.StatusInternalServerError, H{"error": err.Error()})
		return
	}

	respondJSON(w, http.StatusOK, H{
		"status":        "updated",
		"smsdb_enabled": req.SmsdbEnabled,
	})
}

// UpdateWebhookSettings 更新 Webhook 设置
func (h *SettingHandler) UpdateWebhookSettings(w http.ResponseWriter, r *http.Request) {
	var req struct {
		WebhookEnabled bool `json:"webhook_enabled"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondJSON(w, http.StatusBadRequest, H{"error": err.Error()})
		return
	}

	if err := database.SetWebhookEnabled(req.WebhookEnabled); err != nil {
		respondJSON(w, http.StatusInternalServerError, H{"error": err.Error()})
		return
	}

	respondJSON(w, http.StatusOK, H{
		"status":          "updated",
		"webhook_enabled": req.WebhookEnabled,
	})
}
