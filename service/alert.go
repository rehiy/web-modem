package service

import (
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/rehiy/web-modem/database"
	"github.com/rehiy/web-modem/models"
)

// AlertService 告警服务
type AlertService struct {
	modemService *ModemService
}

// NewAlertService 创建告警服务
func NewAlertService() *AlertService {
	return &AlertService{
		modemService: GetModemService(),
	}
}

// ScanModemAlerts 扫描当前 modem 状态并刷新活跃告警。
func (s *AlertService) ScanModemAlerts() error {
	s.modemService.ScanModems()
	conns := s.modemService.GetConnList()
	connectedCount := 0

	for _, conn := range conns {
		if conn == nil {
			continue
		}

		source := conn.Name
		if conn.Device == nil || !conn.State().Connected || !conn.IsOpen() {
			if err := RaiseAlert("modem_offline", "critical", source, "卡离线", fmt.Sprintf("%s 未连接或串口不可用", source)); err != nil {
				log.Printf("[Alert] Failed to raise modem_offline alert: %v", err)
			}
			continue
		}

		connectedCount++
		ResolveAlert("modem_offline", source)
		s.scanSIMStatus(conn)
		s.scanNetworkStatus(conn)
		s.scanSignalStatus(conn)
		s.scanSmsCenterStatus(conn)
	}

	if connectedCount == 0 {
		return RaiseAlert("no_modem", "critical", "system", "未检测到可用卡", "当前没有已连接的 modem")
	}

	ResolveAlert("no_modem", "system")
	return nil
}

func (s *AlertService) scanSIMStatus(conn *ModemConn) {
	status, err := conn.GetSIMStatus()
	source := conn.Name
	if err != nil {
		RaiseAlert("sim_status_unknown", "warning", source, "SIM 状态未知", err.Error())
		return
	}

	ResolveAlert("sim_status_unknown", source)
	if !strings.EqualFold(strings.TrimSpace(status), "READY") {
		RaiseAlert("sim_not_ready", "critical", source, "SIM 未就绪", status)
		return
	}

	ResolveAlert("sim_not_ready", source)
}

func (s *AlertService) scanNetworkStatus(conn *ModemConn) {
	source := conn.Name
	_, stat, err := conn.GetNetworkStatus()
	if err != nil {
		RaiseAlert("network_status_unknown", "warning", source, "网络注册状态未知", err.Error())
		return
	}

	ResolveAlert("network_status_unknown", source)
	if stat != 1 && stat != 5 {
		RaiseAlert("network_not_registered", "critical", source, "网络未注册", registrationAlertDetail(stat))
		ResolveAlert("roaming_active", source)
		return
	}

	ResolveAlert("network_not_registered", source)
	if stat == 5 {
		RaiseAlert("roaming_active", "warning", source, "当前处于漫游", "网络注册状态为漫游")
		return
	}
	ResolveAlert("roaming_active", source)
}

func (s *AlertService) scanSignalStatus(conn *ModemConn) {
	source := conn.Name
	rssi, _, err := conn.GetSignalQuality()
	if err != nil {
		RaiseAlert("signal_unknown", "warning", source, "信号状态未知", err.Error())
		return
	}

	ResolveAlert("signal_unknown", source)
	if rssi == 99 || rssi <= 1 {
		RaiseAlert("no_signal", "critical", source, "无信号", fmt.Sprintf("RSSI=%d", rssi))
		ResolveAlert("low_signal", source)
		return
	}

	ResolveAlert("no_signal", source)
	if rssi <= 6 {
		RaiseAlert("low_signal", "warning", source, "信号弱", fmt.Sprintf("RSSI=%d, 约 %d%%", rssi, min(100, rssi*5)))
		return
	}

	ResolveAlert("low_signal", source)
}

func (s *AlertService) scanSmsCenterStatus(conn *ModemConn) {
	source := conn.Name
	center, _, err := conn.GetSmsCenter()
	if err != nil {
		RaiseAlert("sms_center_missing", "warning", source, "短信中心未知", err.Error())
		ResolveAlert("sms_center_invalid", source)
		return
	}

	center = strings.TrimSpace(center)
	if center == "" {
		RaiseAlert("sms_center_missing", "warning", source, "短信中心缺失", "AT+CSCA? 返回空")
		ResolveAlert("sms_center_invalid", source)
		return
	}

	ResolveAlert("sms_center_missing", source)
	digits := strings.TrimPrefix(center, "+")
	if len(digits) < 5 || normalizeAlertDigits(digits) != digits {
		RaiseAlert("sms_center_invalid", "warning", source, "短信中心格式异常", center)
		return
	}

	ResolveAlert("sms_center_invalid", source)
}

func normalizeAlertDigits(value string) string {
	return strings.Map(func(r rune) rune {
		if r >= '0' && r <= '9' {
			return r
		}
		return -1
	}, value)
}

func registrationAlertDetail(stat int) string {
	switch stat {
	case 0:
		return "未注册"
	case 2:
		return "搜索中"
	case 3:
		return "注册被拒绝"
	case 4:
		return "注册状态未知"
	default:
		return fmt.Sprintf("注册状态: %d", stat)
	}
}

// RaiseAlert 创建或更新活跃告警。
func RaiseAlert(alertType string, severity string, source string, message string, detail string) error {
	alert := &models.Alert{
		Type:     alertType,
		Severity: severity,
		Source:   source,
		Message:  message,
		Detail:   detail,
	}
	created, err := database.UpsertActiveAlert(alert)
	if err != nil {
		return err
	}
	if created {
		NewWebhookService().HandleAlertEvent(WebhookEventAlertTriggered, alert)
	}
	return nil
}

// ResolveAlert 解决同一类型和来源的活跃告警。
func ResolveAlert(alertType string, source string) {
	alert, err := database.ResolveAlertByIdentity(alertType, source)
	if err != nil {
		log.Printf("[Alert] Failed to resolve %s/%s: %v", alertType, source, err)
		return
	}
	if alert != nil {
		ws := NewWebhookService()
		ws.HandleAlertEvent(WebhookEventAlertResolved, alert)
		if alertType == "modem_offline" {
			ws.HandleDataEvent(WebhookEventModemOnline, map[string]any{
				"modem_name": source,
			})
		}
	}
}

// ResolveAlertsByIDs 批量解决告警，并为实际恢复的告警触发 Webhook 事件。
func (s *AlertService) ResolveAlertsByIDs(ids []int) error {
	alerts, err := database.GetActiveAlertsByIDs(ids)
	if err != nil {
		return err
	}
	if err := database.ResolveAlertsByIDs(ids); err != nil {
		return err
	}
	notifyResolvedAlerts(alerts)
	return nil
}

// ResolveAllActiveAlerts 解决全部活跃告警，并为实际恢复的告警触发 Webhook 事件。
func (s *AlertService) ResolveAllActiveAlerts() error {
	alerts, err := database.GetAllActiveAlerts()
	if err != nil {
		return err
	}
	if err := database.ResolveAllActiveAlerts(); err != nil {
		return err
	}
	notifyResolvedAlerts(alerts)
	return nil
}

func notifyResolvedAlerts(alerts []models.Alert) {
	webhookService := NewWebhookService()
	now := time.Now()
	for i := range alerts {
		alerts[i].Status = "resolved"
		alerts[i].ResolvedAt = &now
		webhookService.HandleAlertEvent(WebhookEventAlertResolved, &alerts[i])
	}
}
