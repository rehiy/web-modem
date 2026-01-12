package handler

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/rehiy/web-modem/database"
	"github.com/rehiy/web-modem/models"
	"github.com/rehiy/web-modem/service"
)

// SmsdbHandler 短信存储处理器
type SmsdbHandler struct {
	smsdbService *service.SmsdbService
}

// NewSmsdbHandler 创建新的短信存储处理器
func NewSmsdbHandler() *SmsdbHandler {
	return &SmsdbHandler{
		smsdbService: service.NewSmsdbService(),
	}
}

// ListSMS 获取数据库中的短信列表
func (h *SmsdbHandler) ListSMS(w http.ResponseWriter, r *http.Request) {
	filter := &models.SMSFilter{}

	// 解析查询参数
	if direction := r.URL.Query().Get("direction"); direction != "" {
		filter.Direction = direction
	}

	if sendNumber := r.URL.Query().Get("send_number"); sendNumber != "" {
		filter.SendNumber = sendNumber
	}

	if modemName := r.URL.Query().Get("modem_name"); modemName != "" {
		filter.ModemName = modemName
	}

	if startTime := r.URL.Query().Get("start_time"); startTime != "" {
		if t, err := time.Parse(time.RFC3339, startTime); err == nil {
			filter.StartTime = t
		}
	}

	if endTime := r.URL.Query().Get("end_time"); endTime != "" {
		if t, err := time.Parse(time.RFC3339, endTime); err == nil {
			filter.EndTime = t
		}
	}

	// 分页参数
	filter.Limit = 50 // 默认每页50条
	if limit := r.URL.Query().Get("limit"); limit != "" {
		if l, err := strconv.Atoi(limit); err == nil && l > 0 && l <= 200 {
			filter.Limit = l
		}
	}

	if offset := r.URL.Query().Get("offset"); offset != "" {
		if o, err := strconv.Atoi(offset); err == nil && o >= 0 {
			filter.Offset = o
		}
	}

	smsList, total, err := database.GetSMSList(filter)
	if err != nil {
		respondJSON(w, http.StatusInternalServerError, H{"error": err.Error()})
		return
	}

	respondJSON(w, http.StatusOK, H{
		"data":   smsList,
		"total":  total,
		"limit":  filter.Limit,
		"offset": filter.Offset,
	})
}

// DeleteSMSBatch 批量删除数据库中的短信
func (h *SmsdbHandler) DeleteSMSBatch(w http.ResponseWriter, r *http.Request) {
	var req struct {
		IDs []int `json:"ids"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondJSON(w, http.StatusBadRequest, H{"error": err.Error()})
		return
	}

	if len(req.IDs) == 0 {
		respondJSON(w, http.StatusBadRequest, H{"error": "no IDs provided"})
		return
	}

	if err := database.BatchDeleteSMS(req.IDs); err != nil {
		respondJSON(w, http.StatusInternalServerError, H{"error": err.Error()})
		return
	}

	respondJSON(w, http.StatusOK, H{
		"status": "deleted",
		"count":  len(req.IDs),
	})
}

// SyncSMS 从指定Modem同步短信到数据库
func (h *SmsdbHandler) SyncSMS(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondJSON(w, http.StatusBadRequest, H{"error": err.Error()})
		return
	}

	if req.Name == "" {
		respondJSON(w, http.StatusBadRequest, H{"error": "name is empty"})
		return
	}

	result, err := h.smsdbService.SyncSMSToDB(req.Name)
	if err != nil {
		respondJSON(w, http.StatusInternalServerError, H{"error": err.Error()})
		return
	}

	respondJSON(w, http.StatusOK, result)
}
