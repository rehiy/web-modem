package service

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/rehiy/web-modem/database"
	"github.com/rehiy/web-modem/models"
)

// WebhookService webhook服务
type WebhookService struct{}

// WebhookEvent 描述一次可投递到 Webhook 的业务事件。
type WebhookEvent struct {
	Type      string
	Timestamp time.Time
	Sms       *models.Sms
	Alert     *models.Alert
	Data      map[string]any
	Error     string
}

const (
	WebhookEventAll                   = "all"
	WebhookEventSmsReceived           = "sms_received"
	WebhookEventSmsSentSuccess        = "sms_sent_success"
	WebhookEventSmsSentFailed         = "sms_sent_failed"
	WebhookEventAlertTriggered        = "alert_triggered"
	WebhookEventAlertResolved         = "alert_resolved"
	WebhookEventSmsSyncCompleted      = "sms_sync_completed"
	WebhookEventSystemUpdateCompleted = "system_update_completed"
	WebhookEventModemOnline           = "modem_online"
	WebhookEventIPChanged             = "ip_changed"
)

var (
	webhookCache     []models.Webhook
	webhookCacheTime time.Time
	webhookCacheMux  sync.RWMutex
	cacheTTL         = 30 * time.Second // 缓存30秒
)

const maxWebhookResponseBodyBytes = 8192

// NewWebhookService 创建webhook服务
func NewWebhookService() *WebhookService {
	return &WebhookService{}
}

// NormalizeWebhookEventType 标准化 Webhook 事件类型。
func NormalizeWebhookEventType(eventType string) string {
	switch strings.TrimSpace(eventType) {
	case WebhookEventAll:
		return WebhookEventAll
	case WebhookEventSmsSentSuccess:
		return WebhookEventSmsSentSuccess
	case WebhookEventSmsSentFailed:
		return WebhookEventSmsSentFailed
	case WebhookEventAlertTriggered:
		return WebhookEventAlertTriggered
	case WebhookEventAlertResolved:
		return WebhookEventAlertResolved
	case WebhookEventSmsSyncCompleted:
		return WebhookEventSmsSyncCompleted
	case WebhookEventSystemUpdateCompleted:
		return WebhookEventSystemUpdateCompleted
	case WebhookEventModemOnline:
		return WebhookEventModemOnline
	case WebhookEventIPChanged:
		return WebhookEventIPChanged
	case "", WebhookEventSmsReceived:
		return WebhookEventSmsReceived
	default:
		return WebhookEventSmsReceived
	}
}

// IsKnownWebhookEventType 判断事件类型是否被当前 Webhook 配置支持。
func IsKnownWebhookEventType(eventType string) bool {
	switch strings.TrimSpace(eventType) {
	case WebhookEventAll,
		WebhookEventSmsReceived,
		WebhookEventSmsSentSuccess,
		WebhookEventSmsSentFailed,
		WebhookEventAlertTriggered,
		WebhookEventAlertResolved,
		WebhookEventSmsSyncCompleted,
		WebhookEventSystemUpdateCompleted,
		WebhookEventModemOnline,
		WebhookEventIPChanged:
		return true
	default:
		return false
	}
}

// InvalidateWebhookCache 清空webhook缓存，确保配置变更后立即生效。
func InvalidateWebhookCache() {
	webhookCacheMux.Lock()
	webhookCache = nil
	webhookCacheTime = time.Time{}
	webhookCacheMux.Unlock()
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
	webhooks, err := database.GetEnabledWebhookList()
	if err != nil {
		return nil, fmt.Errorf("failed to get enabled webhooks: %w", err)
	}

	webhookCacheMux.Lock()
	webhookCache = webhooks
	webhookCacheTime = time.Now()
	webhookCacheMux.Unlock()

	return webhooks, nil
}

// TriggerWebhooks 兼容旧短信入口，按收到短信事件触发 Webhook。
func (w *WebhookService) TriggerWebhooks(sms *models.Sms) error {
	return w.TriggerWebhookEvent(WebhookEvent{
		Type: WebhookEventSmsReceived,
		Sms:  sms,
	})
}

// TriggerWebhookEvent 触发所有匹配事件和条件的启用 Webhook。
func (w *WebhookService) TriggerWebhookEvent(event WebhookEvent) error {
	if !database.IsWebhookEnabled() {
		return nil
	}

	event = normalizeWebhookEvent(event)
	webhooks, err := w.getCachedWebhooks()
	if err != nil {
		return fmt.Errorf("failed to get enabled webhooks: %w", err)
	}

	if len(webhooks) == 0 {
		log.Printf("[Webhook] No enabled webhooks found")
		return nil
	}

	var wg sync.WaitGroup
	semaphore := make(chan struct{}, 5) // 限制并发数为5
	matched := 0

	for _, webhook := range webhooks {
		if !w.matchesWebhookEvent(&webhook, &event) {
			continue
		}
		matched++

		wg.Add(1)
		semaphore <- struct{}{}

		go func(wh models.Webhook) {
			defer wg.Done()
			defer func() { <-semaphore }()

			w.triggerWebhook(&wh, &event)
		}(webhook)
	}

	wg.Wait()
	log.Printf("[Webhook] Finished webhook dispatch for %s, matched=%d", event.Type, matched)

	return nil
}

func normalizeWebhookEvent(event WebhookEvent) WebhookEvent {
	event.Type = strings.TrimSpace(event.Type)
	if event.Type == "" {
		if event.Sms != nil {
			event.Type = WebhookEventSmsReceived
		} else {
			event.Type = WebhookEventAll
		}
	}
	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now()
	}
	if event.Data == nil {
		event.Data = map[string]any{}
	}
	return event
}

func (w *WebhookService) matchesWebhookEvent(webhook *models.Webhook, event *WebhookEvent) bool {
	configEventType := NormalizeWebhookEventType(webhook.EventType)
	if configEventType != WebhookEventAll && configEventType != event.Type {
		return false
	}

	return w.matchesEventCondition(webhook, event)
}

func (w *WebhookService) matchesEventCondition(webhook *models.Webhook, event *WebhookEvent) bool {
	matchType := strings.TrimSpace(webhook.MatchType)
	matchValue := strings.TrimSpace(webhook.MatchValue)
	if matchType == "" || matchType == "all" {
		return true
	}
	if matchValue == "" {
		return false
	}

	switch matchType {
	case "receive_number":
		return event.Sms != nil && sameMatchValue(event.Sms.ReceiveNumber, matchValue)
	case "device_imei":
		return event.Sms != nil && sameMatchValue(event.Sms.DeviceIMEI, matchValue)
	case "sim_iccid":
		return event.Sms != nil && sameMatchValue(event.Sms.SimICCID, matchValue)
	case "sim_imsi":
		return event.Sms != nil && sameMatchValue(event.Sms.SimIMSI, matchValue)
	case "operator":
		return event.Sms != nil && sameMatchValue(event.Sms.Operator, matchValue)
	case "send_number":
		return event.Sms != nil && sameMatchValue(event.Sms.SendNumber, matchValue)
	case "modem_name":
		return event.Sms != nil && sameMatchValue(event.Sms.ModemName, matchValue)
	case "direction":
		return event.Sms != nil && sameMatchValue(event.Sms.Direction, strings.ToLower(matchValue))
	case "content_contains":
		return event.Sms != nil && strings.Contains(strings.ToLower(event.Sms.Content), strings.ToLower(matchValue))
	case "alert_type":
		return event.Alert != nil && sameMatchValue(event.Alert.Type, matchValue)
	case "alert_severity":
		return event.Alert != nil && sameMatchValue(event.Alert.Severity, matchValue)
	case "alert_source":
		return event.Alert != nil && sameMatchValue(event.Alert.Source, matchValue)
	default:
		return false
	}
}

func sameMatchValue(actual string, expected string) bool {
	return strings.EqualFold(strings.TrimSpace(actual), strings.TrimSpace(expected))
}

// triggerWebhook 触发单个webhook，支持重试机制
func (w *WebhookService) triggerWebhook(webhook *models.Webhook, event *WebhookEvent) error {
	payload, err := w.preparePayload(webhook, event)
	if err != nil {
		log.Printf("[Webhook] Failed to prepare payload for %s: %v", webhook.Name, err)
		w.recordDelivery(webhook, event, 1, "failed", 0, "", 0, fmt.Errorf("prepare payload: %w", err))
		return err
	}

	totalAttempts, retryDelay, backoff := w.retryConfig(webhook)
	client := &http.Client{Timeout: 30 * time.Second}

	var lastErr error
	delay := retryDelay
	for attempt := 1; attempt <= totalAttempts; attempt++ {
		if attempt > 1 {
			log.Printf("[Webhook] Retry attempt %d for webhook %s", attempt-1, webhook.Name)
			time.Sleep(delay)
			if backoff {
				delay *= 2
			}
		}

		req, err := http.NewRequest("POST", webhook.URL, bytes.NewBuffer(payload))
		if err != nil {
			lastErr = fmt.Errorf("create request: %w", err)
			log.Printf("[Webhook] Failed to create request for %s: %v", webhook.Name, err)
			w.recordDelivery(webhook, event, attempt, "failed", 0, "", 0, lastErr)
			break
		}

		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("User-Agent", "Web-Modem/1.0")
		req.Header.Set("X-Webhook-Event", event.Type)

		start := time.Now()
		resp, err := client.Do(req)
		duration := time.Since(start)

		if err != nil {
			lastErr = fmt.Errorf("send request: %w", err)
			log.Printf("[Webhook] Failed to send request to %s (attempt %d): %v", webhook.Name, attempt, err)
			w.recordDelivery(webhook, event, attempt, "failed", 0, "", duration, lastErr)
			continue
		}

		responseBody, readErr := readWebhookResponseBody(resp.Body)
		if closeErr := resp.Body.Close(); closeErr != nil && readErr == nil {
			readErr = closeErr
		}

		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			if readErr != nil {
				lastErr = fmt.Errorf("read response: %w", readErr)
				w.recordDelivery(webhook, event, attempt, "failed", resp.StatusCode, responseBody, duration, lastErr)
				continue
			}

			w.recordDelivery(webhook, event, attempt, "success", resp.StatusCode, responseBody, duration, nil)
			log.Printf("[Webhook] Successfully triggered %s (event: %s, status: %d, duration: %v)",
				webhook.Name, event.Type, resp.StatusCode, duration)
			return nil
		}

		lastErr = fmt.Errorf("HTTP status %d", resp.StatusCode)
		if readErr != nil {
			lastErr = fmt.Errorf("%w; read response: %v", lastErr, readErr)
		}
		w.recordDelivery(webhook, event, attempt, "failed", resp.StatusCode, responseBody, duration, lastErr)
		log.Printf("[Webhook] Failed to trigger %s (event: %s, status: %d, attempt %d)",
			webhook.Name, event.Type, resp.StatusCode, attempt)

		if resp.StatusCode < 500 || resp.StatusCode >= 600 {
			break
		}
	}

	log.Printf("[Webhook] All %d attempts failed for webhook %s", totalAttempts, webhook.Name)
	if lastErr == nil {
		lastErr = fmt.Errorf("unknown webhook delivery error")
	}
	return fmt.Errorf("failed to trigger webhook %s after %d attempts: %w", webhook.Name, totalAttempts, lastErr)
}

func (w *WebhookService) retryConfig(webhook *models.Webhook) (int, time.Duration, bool) {
	retryCount := webhook.RetryCount
	if retryCount < 0 {
		retryCount = 0
	}
	if retryCount > 10 {
		retryCount = 10
	}

	intervalSeconds := webhook.RetryIntervalSeconds
	if intervalSeconds <= 0 {
		intervalSeconds = 2
	}
	if intervalSeconds > 3600 {
		intervalSeconds = 3600
	}

	return retryCount + 1, time.Duration(intervalSeconds) * time.Second, webhook.RetryBackoff
}

func (w *WebhookService) recordDelivery(webhook *models.Webhook, event *WebhookEvent, attempt int, status string, httpStatusCode int, responseBody string, duration time.Duration, failure error) {
	errorText := ""
	if failure != nil {
		errorText = failure.Error()
	}

	delivery := &models.WebhookDelivery{
		WebhookID:      webhook.ID,
		WebhookName:    webhook.Name,
		URL:            webhook.URL,
		EventType:      event.Type,
		SmsID:          eventSmsID(event),
		AlertID:        eventAlertID(event),
		Status:         status,
		HTTPStatusCode: httpStatusCode,
		ResponseBody:   responseBody,
		DurationMs:     duration.Milliseconds(),
		Error:          errorText,
		Attempt:        attempt,
	}

	if err := database.CreateWebhookDelivery(delivery); err != nil {
		log.Printf("[Webhook] Failed to record delivery for %s: %v", webhook.Name, err)
	}

	source := webhookAlertSource(webhook)
	if status == "success" {
		ResolveAlert("webhook_failed", source)
		return
	}
	if status == "failed" {
		detail := errorText
		if detail == "" {
			detail = responseBody
		}
		if err := RaiseAlert("webhook_failed", "warning", source, "Webhook 发送失败", detail); err != nil {
			log.Printf("[Webhook] Failed to raise webhook alert for %s: %v", webhook.Name, err)
		}
	}
}

func eventSmsID(event *WebhookEvent) int {
	if event != nil && event.Sms != nil {
		return event.Sms.ID
	}
	return 0
}

func eventAlertID(event *WebhookEvent) int {
	if event != nil && event.Alert != nil {
		return event.Alert.ID
	}
	return 0
}

func readWebhookResponseBody(body io.Reader) (string, error) {
	data, err := io.ReadAll(io.LimitReader(body, maxWebhookResponseBodyBytes+1))
	if len(data) > maxWebhookResponseBodyBytes {
		return string(data[:maxWebhookResponseBodyBytes]) + "\n... response truncated", err
	}
	return string(data), err
}

func webhookAlertSource(webhook *models.Webhook) string {
	if webhook.ID > 0 {
		return fmt.Sprintf("webhook:%d", webhook.ID)
	}
	if webhook.Name != "" {
		return "webhook:" + webhook.Name
	}
	return "webhook:" + webhook.URL
}

// preparePayload 准备webhook payload
func (w *WebhookService) preparePayload(webhook *models.Webhook, event *WebhookEvent) ([]byte, error) {
	if webhook.Template == "" || webhook.Template == "{}" {
		return w.getDefaultPayload(event)
	}

	var template any
	if err := json.Unmarshal([]byte(webhook.Template), &template); err != nil {
		return nil, fmt.Errorf("invalid webhook template: %w", err)
	}

	payload := w.replaceTemplateValue(template, event)

	return json.Marshal(payload)
}

// getDefaultPayload 获取默认payload
func (w *WebhookService) getDefaultPayload(event *WebhookEvent) ([]byte, error) {
	payload := map[string]any{
		"event":      event.Type,
		"data":       w.eventData(event),
		"timestamp":  event.Timestamp.Unix(),
		"event_time": event.Timestamp.Format(time.RFC3339),
	}

	return json.Marshal(payload)
}

func (w *WebhookService) eventData(event *WebhookEvent) map[string]any {
	data := map[string]any{}
	for key, value := range event.Data {
		data[key] = value
	}

	if event.Sms != nil {
		for key, value := range smsPayloadFields(event.Sms) {
			data[key] = value
		}
	}
	if event.Alert != nil {
		for key, value := range alertPayloadFields(event.Alert) {
			data[key] = value
		}
	}
	if event.Error != "" {
		data["error"] = event.Error
	}

	return data
}

func smsPayloadFields(sms *models.Sms) map[string]any {
	return map[string]any{
		"id":             sms.ID,
		"content":        sms.Content,
		"sms_ids":        sms.SmsIDs,
		"receive_time":   sms.ReceiveTime.Format(time.RFC3339),
		"receive_number": sms.ReceiveNumber,
		"send_number":    sms.SendNumber,
		"direction":      sms.Direction,
		"modem_name":     sms.ModemName,
		"device_imei":    sms.DeviceIMEI,
		"sim_iccid":      sms.SimICCID,
		"sim_imsi":       sms.SimIMSI,
		"operator":       sms.Operator,
	}
}

func alertPayloadFields(alert *models.Alert) map[string]any {
	fields := map[string]any{
		"id":         alert.ID,
		"type":       alert.Type,
		"severity":   alert.Severity,
		"status":     alert.Status,
		"source":     alert.Source,
		"message":    alert.Message,
		"detail":     alert.Detail,
		"count":      alert.Count,
		"first_seen": alert.FirstSeen.Format(time.RFC3339),
		"last_seen":  alert.LastSeen.Format(time.RFC3339),
	}
	if alert.ResolvedAt != nil {
		fields["resolved_at"] = alert.ResolvedAt.Format(time.RFC3339)
	} else {
		fields["resolved_at"] = ""
	}
	return fields
}

// replaceTemplateVariables 替换模板中的变量
func (w *WebhookService) replaceTemplateVariables(template map[string]any, event *WebhookEvent) map[string]any {
	result := make(map[string]any)

	for key, value := range template {
		result[key] = w.replaceTemplateValue(value, event)
	}

	return result
}

func (w *WebhookService) replaceTemplateValue(value any, event *WebhookEvent) any {
	switch v := value.(type) {
	case string:
		return w.replaceStringVariables(v, event)
	case map[string]any:
		return w.replaceTemplateVariables(v, event)
	case []any:
		result := make([]any, len(v))
		for i, item := range v {
			result[i] = w.replaceTemplateValue(item, event)
		}
		return result
	default:
		return value
	}
}

// replaceStringVariables 替换字符串中的变量
func (w *WebhookService) replaceStringVariables(s string, event *WebhookEvent) string {
	for old, new := range w.templateReplacements(event) {
		s = strings.ReplaceAll(s, old, new)
	}

	return s
}

func (w *WebhookService) templateReplacements(event *WebhookEvent) map[string]string {
	replacements := map[string]string{
		"{{event}}":      event.Type,
		"{{event_type}}": event.Type,
		"{{event_time}}": event.Timestamp.Format(time.RFC3339),
		"{{timestamp}}":  strconv.FormatInt(event.Timestamp.Unix(), 10),
		"{{error}}":      event.Error,
	}

	if event.Sms != nil {
		for key, value := range smsPayloadFields(event.Sms) {
			replacements["{{"+key+"}}"] = templateString(value)
		}
	}
	if event.Alert != nil {
		for key, value := range alertPayloadFields(event.Alert) {
			replacements["{{alert_"+key+"}}"] = templateString(value)
		}
	}
	for key, value := range event.Data {
		replacements["{{"+key+"}}"] = templateString(value)
	}

	return replacements
}

func templateString(value any) string {
	switch v := value.(type) {
	case nil:
		return ""
	case string:
		return v
	case time.Time:
		return v.Format(time.RFC3339)
	default:
		data, err := json.Marshal(v)
		if err == nil {
			return strings.Trim(string(data), `"`)
		}
		return fmt.Sprint(v)
	}
}

func sampleWebhookEvent(eventType string) WebhookEvent {
	now := time.Now()
	switch NormalizeWebhookEventType(eventType) {
	case WebhookEventSmsSentSuccess:
		return WebhookEvent{
			Type:      WebhookEventSmsSentSuccess,
			Timestamp: now,
			Sms:       sampleWebhookSms("out"),
		}
	case WebhookEventSmsSentFailed:
		return WebhookEvent{
			Type:      WebhookEventSmsSentFailed,
			Timestamp: now,
			Sms:       sampleWebhookSms("out"),
			Error:     "send request timeout",
		}
	case WebhookEventAlertTriggered:
		return WebhookEvent{
			Type:      WebhookEventAlertTriggered,
			Timestamp: now,
			Alert:     sampleWebhookAlert("active", nil),
		}
	case WebhookEventAlertResolved:
		return WebhookEvent{
			Type:      WebhookEventAlertResolved,
			Timestamp: now,
			Alert:     sampleWebhookAlert("resolved", &now),
		}
	case WebhookEventSmsSyncCompleted:
		return WebhookEvent{
			Type:      WebhookEventSmsSyncCompleted,
			Timestamp: now,
			Data: map[string]any{
				"sync_scope":    "all",
				"total_modems":  2,
				"success_count": 2,
				"failed_count":  0,
				"total_count":   18,
				"new_count":     3,
			},
		}
	case WebhookEventSystemUpdateCompleted:
		return WebhookEvent{
			Type:      WebhookEventSystemUpdateCompleted,
			Timestamp: now,
			Data: map[string]any{
				"status":          "updated",
				"current_version": "1.0.0",
				"latest_version":  "1.1.0",
				"restart":         true,
			},
		}
	case WebhookEventModemOnline:
		return WebhookEvent{
			Type:      WebhookEventModemOnline,
			Timestamp: now,
			Data: map[string]any{
				"modem_name": "ttyUSB2",
			},
		}
	case WebhookEventIPChanged:
		return WebhookEvent{
			Type:      WebhookEventIPChanged,
			Timestamp: now,
			Data: map[string]any{
				"old_ips":     "192.168.1.100",
				"new_ips":     "10.0.0.50",
				"added_ips":   "10.0.0.50",
				"removed_ips": "192.168.1.100",
			},
		}
	default:
		return WebhookEvent{
			Type:      WebhookEventSmsReceived,
			Timestamp: now,
			Sms:       sampleWebhookSms("in"),
		}
	}
}

func sampleWebhookSms(direction string) *models.Sms {
	sms := &models.Sms{
		ID:          0,
		Content:     "Test webhook message",
		SmsIDs:      "1,2,3",
		ReceiveTime: time.Now(),
		Direction:   direction,
		ModemName:   "ttyUSB2",
		DeviceIMEI:  "000000000000000",
		SimICCID:    "89860000000000000000",
		SimIMSI:     "460001234567890",
		Operator:    "46000",
	}
	if direction == "out" {
		sms.ReceiveNumber = "+8613800138001"
		sms.SendNumber = "+8613800138000"
	} else {
		sms.ReceiveNumber = "+8613800138000"
		sms.SendNumber = "+8613800138001"
	}
	return sms
}

func sampleWebhookAlert(status string, resolvedAt *time.Time) *models.Alert {
	now := time.Now()
	return &models.Alert{
		ID:         0,
		Type:       "low_signal",
		Severity:   "warning",
		Status:     status,
		Source:     "ttyUSB2",
		Message:    "信号弱",
		Detail:     "RSSI=6, 约 30%",
		Count:      1,
		FirstSeen:  now.Add(-2 * time.Minute),
		LastSeen:   now,
		ResolvedAt: resolvedAt,
	}
}

// PreviewPayload 渲染webhook预览payload
func (w *WebhookService) PreviewPayload(webhook *models.Webhook) ([]byte, error) {
	event := sampleWebhookEvent(webhook.EventType)
	return w.preparePayload(webhook, &event)
}

// TestWebhook 测试webhook
func (w *WebhookService) TestWebhook(webhook *models.Webhook) error {
	event := sampleWebhookEvent(webhook.EventType)
	return w.triggerWebhook(webhook, &event)
}

// HandleEvent 异步处理通用 Webhook 事件。
func (w *WebhookService) HandleEvent(event WebhookEvent) {
	go func() {
		defer func() {
			if r := recover(); r != nil {
				log.Printf("[Webhook] Panic recovered: %v", r)
			}
		}()
		if err := w.TriggerWebhookEvent(event); err != nil {
			log.Printf("[Webhook] Failed to trigger webhooks: %v", err)
		}
	}()
}

// HandleIncomingSms 处理接收到的短信：触发 webhook
func (w *WebhookService) HandleIncomingSms(dbSms *models.Sms) {
	w.HandleEvent(WebhookEvent{
		Type: WebhookEventSmsReceived,
		Sms:  dbSms,
	})
}

// HandleSmsSentSuccess 处理发送短信成功事件。
func (w *WebhookService) HandleSmsSentSuccess(sms *models.Sms) {
	w.HandleEvent(WebhookEvent{
		Type: WebhookEventSmsSentSuccess,
		Sms:  sms,
	})
}

// HandleSmsSentFailed 处理发送短信失败事件。
func (w *WebhookService) HandleSmsSentFailed(sms *models.Sms, failure error) {
	event := WebhookEvent{
		Type: WebhookEventSmsSentFailed,
		Sms:  sms,
	}
	if failure != nil {
		event.Error = failure.Error()
	}
	w.HandleEvent(event)
}

// HandleAlertEvent 处理告警触发/恢复事件。
func (w *WebhookService) HandleAlertEvent(eventType string, alert *models.Alert) {
	if alert == nil || alert.Type == "webhook_failed" {
		return
	}
	w.HandleEvent(WebhookEvent{
		Type:  eventType,
		Alert: alert,
	})
}

// HandleDataEvent 处理无短信/告警对象的系统事件。
func (w *WebhookService) HandleDataEvent(eventType string, data map[string]any) {
	w.HandleEvent(WebhookEvent{
		Type: eventType,
		Data: data,
	})
}
