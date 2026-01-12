package handler

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/rehiy/web-modem/database"
	"github.com/rehiy/web-modem/models"
	"github.com/rehiy/web-modem/service"
)

// WebhookHandler Webhook处理器
type WebhookHandler struct {
	ws *service.WebhookService
}

// NewWebhookHandler 创建新的Webhook处理器
func NewWebhookHandler() *WebhookHandler {
	return &WebhookHandler{
		ws: service.NewWebhookService(),
	}
}

// CreateWebhook 创建Webhook配置
func (h *WebhookHandler) CreateWebhook(w http.ResponseWriter, r *http.Request) {
	var webhook models.Webhook
	if err := json.NewDecoder(r.Body).Decode(&webhook); err != nil {
		respondJSON(w, http.StatusBadRequest, H{"error": err.Error()})
		return
	}

	// 验证必填字段
	if webhook.Name == "" || webhook.URL == "" {
		respondJSON(w, http.StatusBadRequest, H{"error": "name and url are required"})
		return
	}

	// 如果模板为空，使用默认模板
	if webhook.Template == "" {
		webhook.Template = "{}"
	}

	if err := database.CreateWebhook(&webhook); err != nil {
		respondJSON(w, http.StatusInternalServerError, H{"error": err.Error()})
		return
	}

	respondJSON(w, http.StatusCreated, webhook)
}

// UpdateWebhook 更新Webhook配置
func (h *WebhookHandler) UpdateWebhook(w http.ResponseWriter, r *http.Request) {
	vars := r.URL.Query()
	idStr := vars.Get("id")
	if idStr == "" {
		respondJSON(w, http.StatusBadRequest, H{"error": "id is required"})
		return
	}

	id, err := strconv.Atoi(idStr)
	if err != nil {
		respondJSON(w, http.StatusBadRequest, H{"error": "invalid id"})
		return
	}

	var webhook models.Webhook
	if err := json.NewDecoder(r.Body).Decode(&webhook); err != nil {
		respondJSON(w, http.StatusBadRequest, H{"error": err.Error()})
		return
	}

	webhook.ID = id

	// 验证必填字段
	if webhook.Name == "" || webhook.URL == "" {
		respondJSON(w, http.StatusBadRequest, H{"error": "name and url are required"})
		return
	}

	// 如果模板为空，使用默认模板
	if webhook.Template == "" {
		webhook.Template = "{}"
	}

	if err := database.UpdateWebhook(&webhook); err != nil {
		respondJSON(w, http.StatusInternalServerError, H{"error": err.Error()})
		return
	}

	respondJSON(w, http.StatusOK, webhook)
}

// DeleteWebhook 删除Webhook配置
func (h *WebhookHandler) DeleteWebhook(w http.ResponseWriter, r *http.Request) {
	vars := r.URL.Query()
	idStr := vars.Get("id")
	if idStr == "" {
		respondJSON(w, http.StatusBadRequest, H{"error": "id is required"})
		return
	}

	id, err := strconv.Atoi(idStr)
	if err != nil {
		respondJSON(w, http.StatusBadRequest, H{"error": "invalid id"})
		return
	}

	if err := database.DeleteWebhook(id); err != nil {
		respondJSON(w, http.StatusInternalServerError, H{"error": err.Error()})
		return
	}

	respondJSON(w, http.StatusOK, H{
		"status": "deleted",
		"id":     id,
	})
}

// GetWebhook 获取单个Webhook配置
func (h *WebhookHandler) GetWebhook(w http.ResponseWriter, r *http.Request) {
	vars := r.URL.Query()
	idStr := vars.Get("id")
	if idStr == "" {
		respondJSON(w, http.StatusBadRequest, H{"error": "id is required"})
		return
	}

	id, err := strconv.Atoi(idStr)
	if err != nil {
		respondJSON(w, http.StatusBadRequest, H{"error": "invalid id"})
		return
	}

	webhook, err := database.GetWebhook(id)
	if err != nil {
		respondJSON(w, http.StatusNotFound, H{"error": err.Error()})
		return
	}

	respondJSON(w, http.StatusOK, webhook)
}

// ListWebhooks 获取所有Webhook配置
func (h *WebhookHandler) ListWebhooks(w http.ResponseWriter, r *http.Request) {
	webhooks, err := database.GetWebhookList()
	if err != nil {
		respondJSON(w, http.StatusInternalServerError, H{"error": err.Error()})
		return
	}

	respondJSON(w, http.StatusOK, webhooks)
}

// TestWebhook 测试Webhook
func (h *WebhookHandler) TestWebhook(w http.ResponseWriter, r *http.Request) {
	vars := r.URL.Query()
	idStr := vars.Get("id")
	if idStr == "" {
		respondJSON(w, http.StatusBadRequest, H{"error": "id is required"})
		return
	}

	id, err := strconv.Atoi(idStr)
	if err != nil {
		respondJSON(w, http.StatusBadRequest, H{"error": "invalid id"})
		return
	}

	webhook, err := database.GetWebhook(id)
	if err != nil {
		respondJSON(w, http.StatusNotFound, H{"error": err.Error()})
		return
	}

	if err := h.ws.TestWebhook(webhook); err != nil {
		respondJSON(w, http.StatusInternalServerError, H{"error": err.Error()})
		return
	}

	respondJSON(w, http.StatusOK, H{
		"status":  "success",
		"message": "Webhook test sent successfully",
	})
}
