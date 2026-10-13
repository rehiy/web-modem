package service

import (
	"fmt"
	"log"
	"time"

	"github.com/rehiy/web-modem/database"
)

// 短信兜底轮询：收信主链路依赖模组上报 +CMTI URC，一旦通知丢失（CNMI 配置
// 失效、URC 被丢弃、程序离线期间收到短信等），短信会滞留在设备存储中且不触发
// webhook；收件存储写满后模组无法再存入新短信，收信从此静默而入网状态依旧
// 正常。轮询周期性读取设备上的全部短信，补录漏掉的短信并清理设备存储。

// smsStorageAlertRatio 收件存储占用率告警阈值。
const smsStorageAlertRatio = 0.8

// StartSmsPollWatch 启动短信兜底轮询。
func StartSmsPollWatch(interval time.Duration) {
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for range ticker.C {
			PollAllModemSms()
		}
	}()
}

// PollAllModemSms 轮询所有已连接 modem 的设备短信。
func PollAllModemSms() {
	m := GetModemService()
	for _, name := range m.GetConnectedNames() {
		if err := m.pollModemSms(name); err != nil {
			log.Printf("[%s] sms poll failed: %v", name, err)
		}
	}
}

// pollModemSms 读取单个 modem 设备存储中的全部短信：未入库的按新短信补录并
// 触发 webhook，已入库的直接删除（历史手动同步不清理设备），避免存储写满。
func (m *ModemService) pollModemSms(portName string) error {
	conn, err := m.GetConn(portName)
	if err != nil {
		return err
	}

	// 与 +CMTI 事件处理互斥，避免同一条短信被两条链路重复处理
	conn.smsMu.Lock()
	defer conn.smsMu.Unlock()

	smsList, err := conn.ListSmsPdu(4)
	if err != nil {
		return fmt.Errorf("list sms: %w", err)
	}

	smsdbService := NewSmsdbService()
	webhookService := NewWebhookService()

	for _, atSms := range smsList {
		modelSms := atSmsToModelSms(atSms, conn)

		// 短信库启用时先查重：+CMTI 链路已处理过、或历史同步已入库的短信
		// 不再重复触发 webhook，仅清理设备存储
		exists := false
		if database.IsSmsdbEnabled() {
			exists, err = FindExistingDeviceSms(atSms, modelSms)
			if err != nil {
				log.Printf("[%s] sms poll dedupe failed, skip indices %v: %v", portName, atSms.Indices, err)
				continue
			}
		}

		if !exists {
			log.Printf("[%s] sms poll recovered missed Sms from %s: %s", portName, atSms.Number, atSms.Text)
			smsdbService.HandleIncomingSms(modelSms)
			webhookService.HandleIncomingSms(modelSms)
		}

		if err := conn.DeleteSms(atSms.Indices); err != nil {
			log.Printf("[%s] sms poll failed to delete Sms %v: %v", portName, atSms.Indices, err)
		}
	}

	m.checkSmsStorage(conn)
	return nil
}

// checkSmsStorage 检查设备收件存储占用，接近写满时告警。
// 写满后模组无法存入新短信，也不会再上报 +CMTI，收信会静默失败；
// 占用长期偏高通常意味着设备上滞留了无法合并的长短信残缺分片。
func (m *ModemService) checkSmsStorage(conn *ModemConn) {
	store, err := conn.GetSmsStore()
	if err != nil {
		return
	}

	used, usedOk := store["used3"].(int)
	total, totalOk := store["total3"].(int)
	if !usedOk || !totalOk || total <= 0 {
		return
	}

	source := conn.Name
	if float64(used) >= float64(total)*smsStorageAlertRatio {
		detail := fmt.Sprintf("收件存储已用 %d/%d，写满后将无法接收新短信", used, total)
		if err := RaiseAlert("sms_storage_full", "warning", source, "短信存储接近写满", detail); err != nil {
			log.Printf("[%s] failed to raise sms_storage_full alert: %v", source, err)
		}
		return
	}
	ResolveAlert("sms_storage_full", source)
}
