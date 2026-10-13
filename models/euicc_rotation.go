package models

import "time"

// EuiccRotation 存储每个 eUICC 设备的 Profile 轮询配置。
//
// 一张 eUICC 同一时刻只能有一个 Profile 启用，无法并发接收多个号码的短信。
// 轮询调度周期性地依次切换到每张 Profile，从运营商短信中心拉回排队暂存的
// 短信，再切回主卡（PrimaryICCID）。按设备名（ModemName）唯一。
type EuiccRotation struct {
	ModemName    string    `json:"modemName" gorm:"primaryKey;column:modem_name"`
	Enabled      bool      `json:"enabled" gorm:"column:enabled"`
	IntervalMin  int       `json:"intervalMin" gorm:"column:interval_min"`             // 轮询间隔（分钟），两轮之间的最小间隔
	PrimaryICCID string    `json:"primaryIccid" gorm:"column:primary_iccid;type:text"` // 主卡 ICCID，轮询结束后切回；为空取当前启用卡
	LastRunAt    time.Time `json:"lastRunAt" gorm:"column:last_run_at"`                // 上次完整轮询结束时间，调度据此判断是否到点
	UpdatedAt    time.Time `json:"updatedAt" gorm:"autoUpdateTime"`
}
