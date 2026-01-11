package service

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/rehiy/modem/at"
	"github.com/rehiy/web-modem/database"
	"github.com/rehiy/web-modem/models"
)

// WebhookService webhook服务
type WebhookService struct{}

var (
	webhookCache     []models.Webhook
	webhookCacheTime time.Time
	webhookCacheMux  sync.RWMutex
	cacheTTL         = 30 * time.Second // 缓存30秒
)

// atSMSToModelSMS 将 at.SMS 转换为 models.SMS
func atSMSToModelSMS(smsData at.SMS, receiveNumber string) *models.SMS {
	return &models.SMS{
		Content:       smsData.Text,
		SMSIDs:        database.IntArrayToString(smsData.Indices),
		ReceiveTime:   parseSMSTime(smsData.Time),
		ReceiveNumber: receiveNumber,
		SendNumber:    smsData.PhoneNumber,
		Direction:     "in",
	}
}

// parseSMSTime 解析短信时间字符串
func parseSMSTime(timeStr string) time.Time {
	if timeStr == "" {
		return time.Now()
	}

	// 尝试解析常见的短信时间格式
	formats := []string{
		"2006/01/02 15:04:05",
		"2006-01-02 15:04:05",
		"02/01/06 15:04:05",
	}

	for _, format := range formats {
		if t, err := time.Parse(format, timeStr); err == nil {
			return t
		}
	}

	// 如果无法解析，返回当前时间
	return time.Now()
}

// NewWebhookService 创建webhook服务
func NewWebhookService() *WebhookService {
	return &WebhookService{}
}

// getCachedWebhooks 获取缓存的webhook列表
func (w *WebhookService) getCachedWebhooks() ([]models.Webhook, error) {
	webhookCacheMux.RLock()
	if time.Since(webhookCacheTime) < cacheTTL && len(webhookCache) > 0 {
		webhooks := webhookCache
		webhookCacheMux.RUnlock()
		return webhooks, nil
	}
	webhookCacheMux.RUnlock()

	// 缓存过期或为空，重新查询
	webhooks, err := database.GetEnabledWebhooks()
	if err != nil {
		return nil, fmt.Errorf("failed to get enabled webhooks: %w", err)
	}

	webhookCacheMux.Lock()
	webhookCache = webhooks
	webhookCacheTime = time.Now()
	webhookCacheMux.Unlock()

	return webhooks, nil
}

// TriggerWebhooks 触发所有启用的webhook
func (w *WebhookService) TriggerWebhooks(sms *models.SMS) error {
	if !database.IsWebhookEnabled() {
		return nil
	}

	webhooks, err := w.getCachedWebhooks()
	if err != nil {
		return fmt.Errorf("failed to get enabled webhooks: %w", err)
	}

	if len(webhooks) == 0 {
		log.Printf("[Webhook] No enabled webhooks found")
		return nil
	}

	// 使用并发控制触发webhook
	var wg sync.WaitGroup
	semaphore := make(chan struct{}, 5) // 限制并发数为5

	for _, webhook := range webhooks {
		wg.Add(1)
		semaphore <- struct{}{} // 获取信号量

		go func(wh models.Webhook) {
			defer wg.Done()
			defer func() { <-semaphore }() // 释放信号量

			w.triggerWebhook(&wh, sms)
		}(webhook)
	}

	wg.Wait()
	log.Printf("[Webhook] Successfully triggered %d webhooks for SMS", len(webhooks))

	return nil
}

// triggerWebhook 触发单个webhook，支持重试机制
func (w *WebhookService) triggerWebhook(webhook *models.Webhook, sms *models.SMS) error {
	maxRetries := 3
	retryDelay := 2 * time.Second

	for attempt := 0; attempt < maxRetries; attempt++ {
		if attempt > 0 {
			log.Printf("[Webhook] Retry attempt %d for webhook %s", attempt, webhook.Name)
			time.Sleep(retryDelay)
			retryDelay *= 2 // 指数退避
		}

		// 准备payload
		payload, err := w.preparePayload(webhook, sms)
		if err != nil {
			log.Printf("[Webhook] Failed to prepare payload for %s: %v", webhook.Name, err)
			return err // 模板错误不重试
		}

		// 发送请求
		client := &http.Client{
			Timeout: 30 * time.Second,
		}

		req, err := http.NewRequest("POST", webhook.URL, bytes.NewBuffer(payload))
		if err != nil {
			log.Printf("[Webhook] Failed to create request for %s: %v", webhook.Name, err)
			continue // 请求创建错误重试
		}

		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("User-Agent", "Web-Modem/1.0")

		start := time.Now()
		resp, err := client.Do(req)
		duration := time.Since(start)

		if err != nil {
			log.Printf("[Webhook] Failed to send request to %s (attempt %d): %v", webhook.Name, attempt+1, err)
			continue // 网络错误重试
		}

		defer resp.Body.Close()

		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			log.Printf("[Webhook] Successfully triggered %s (status: %d, duration: %v)",
				webhook.Name, resp.StatusCode, duration)
			return nil
		} else {
			log.Printf("[Webhook] Failed to trigger %s (status: %d, attempt %d)",
				webhook.Name, resp.StatusCode, attempt+1)

			// 如果是服务器错误(5xx)，重试
			if resp.StatusCode >= 500 && resp.StatusCode < 600 {
				continue
			}
			// 如果是客户端错误(4xx)，不重试
			break
		}
	}

	log.Printf("[Webhook] All %d attempts failed for webhook %s", maxRetries, webhook.Name)
	return fmt.Errorf("failed to trigger webhook %s after %d attempts", webhook.Name, maxRetries)
}

// preparePayload 准备webhook payload
func (w *WebhookService) preparePayload(webhook *models.Webhook, sms *models.SMS) ([]byte, error) {
	// 如果template为空或不是有效的JSON，使用默认模板
	if webhook.Template == "" || webhook.Template == "{}" {
		return w.getDefaultPayload(sms)
	}

	// 尝试解析模板
	var template map[string]interface{}
	if err := json.Unmarshal([]byte(webhook.Template), &template); err != nil {
		// 如果模板解析失败，使用默认模板
		log.Printf("[Webhook] Invalid template for %s, using default: %v", webhook.Name, err)
		return w.getDefaultPayload(sms)
	}

	// 替换模板中的变量
	payload := w.replaceTemplateVariables(template, sms)

	return json.Marshal(payload)
}

// getDefaultPayload 获取默认payload
func (w *WebhookService) getDefaultPayload(sms *models.SMS) ([]byte, error) {
	payload := map[string]interface{}{
		"event": "sms_received",
		"data": map[string]interface{}{
			"id":             sms.ID,
			"content":        sms.Content,
			"sms_ids":        sms.SMSIDs,
			"receive_time":   sms.ReceiveTime.Format(time.RFC3339),
			"receive_number": sms.ReceiveNumber,
			"send_number":    sms.SendNumber,
			"direction":      sms.Direction,
		},
		"timestamp": time.Now().Unix(),
	}

	return json.Marshal(payload)
}

// replaceTemplateVariables 替换模板中的变量
func (w *WebhookService) replaceTemplateVariables(template map[string]interface{}, sms *models.SMS) map[string]interface{} {
	result := make(map[string]interface{})

	for key, value := range template {
		switch v := value.(type) {
		case string:
			result[key] = w.replaceStringVariables(v, sms)
		case map[string]interface{}:
			result[key] = w.replaceTemplateVariables(v, sms)
		default:
			result[key] = value
		}
	}

	return result
}

// replaceStringVariables 替换字符串中的变量
func (w *WebhookService) replaceStringVariables(s string, sms *models.SMS) string {
	replacements := map[string]string{
		"{{content}}":        sms.Content,
		"{{sms_ids}}":        sms.SMSIDs,
		"{{receive_time}}":   sms.ReceiveTime.Format(time.RFC3339),
		"{{receive_number}}": sms.ReceiveNumber,
		"{{send_number}}":    sms.SendNumber,
		"{{direction}}":      sms.Direction,
	}

	for old, new := range replacements {
		s = strings.ReplaceAll(s, old, new)
	}

	return s
}

// Test 测试webhook
func (w *WebhookService) Test(webhook *models.Webhook) error {
	testSMS := &models.SMS{
		ID:            0,
		Content:       "Test webhook message",
		SMSIDs:        "1,2,3",
		ReceiveTime:   time.Now(),
		ReceiveNumber: "+8613800138000",
		SendNumber:    "+8613800138001",
		Direction:     "in",
	}

	return w.triggerWebhook(webhook, testSMS)
}

// HandleIncomingSMS 处理接收到的短信，保存到数据库并异步触发webhook
func (w *WebhookService) HandleIncomingSMS(smsData *models.SMS) error {
	// 异步处理，避免阻塞短信接收
	go func() {
		defer func() {
			if r := recover(); r != nil {
				log.Printf("[SMS] Panic recovered in HandleIncomingSMS: %v", r)
			}
		}()

		// 保存到数据库
		sms, err := database.SaveIncomingSMS(smsData)
		if err != nil {
			log.Printf("[SMS] Failed to save incoming SMS: %v", err)
		}

		// 如果webhook启用，触发webhook
		if database.IsWebhookEnabled() {
			var smsForWebhook *models.SMS
			if sms != nil {
				smsForWebhook = sms
			} else {
				// 如果保存失败或未启用，尝试查询
				smsList, err := database.GetsmsdbBodyBySMSIDs(parseSMSIDs(smsData.SMSIDs))
				if err != nil {
					log.Printf("[SMS] Failed to get saved SMS: %v", err)
					return
				}
				if len(smsList) > 0 {
					smsForWebhook = &smsList[0]
				} else {
					// 没有保存的短信，使用传入的SMS对象
					smsForWebhook = smsData
				}
			}

			// 异步触发webhook，不阻塞主流程
			go func() {
				if err := w.TriggerWebhooks(smsForWebhook); err != nil {
					log.Printf("[Webhook] Failed to trigger webhooks: %v", err)
				}
			}()
		}
	}()

	return nil
}

// parseSMSIDs 将字符串形式的SMS IDs解析为int数组
func parseSMSIDs(smsIDsStr string) []int {
	if smsIDsStr == "" {
		return []int{}
	}

	parts := strings.Split(smsIDsStr, ",")
	var ids []int
	for _, part := range parts {
		if id, err := strconv.Atoi(strings.TrimSpace(part)); err == nil {
			ids = append(ids, id)
		}
	}
	return ids
}
