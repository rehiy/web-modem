package service

import (
	"log"
	"sync/atomic"
	"time"

	"github.com/rehiy/web-modem/database"
)

// 轮询单张 Profile 后等待短信中心投递排队短信的时间。
// 模块注册网络后，短信中心会尽快把暂存短信下发，留出窗口兜底收取。
const (
	rotationRegWait        = 90 * time.Second // 切卡后等待网络注册的最长时间
	rotationSmsSettle      = 15 * time.Second // 注册成功后再留时间收取排队短信
	RotationMinIntervalMin = 10               // 最小轮询间隔（分钟），低于此值会被忽略
	rotationTickInterval   = 1 * time.Minute  // 调度器检查间隔
)

// WaitForRegistration 轮询网络注册状态，stat 为 1(已注册)/5(漫游注册) 即视为已驻网。
func WaitForRegistration(conn *ModemConn, maxWait time.Duration) bool {
	deadline := time.Now().Add(maxWait)
	for time.Now().Before(deadline) {
		if _, stat, err := conn.GetNetworkStatus(); err == nil && (stat == 1 || stat == 5) {
			return true
		}
		time.Sleep(3 * time.Second)
	}
	return false
}

// RunEuiccRotation 对指定设备执行一轮 Profile 轮询：
// 依次启用除主卡外的每张 Profile，等待驻网并收取短信中心暂存的短信，
// 最后切回主卡。期间任何一步失败都记录并继续，保证最终回到主卡。
//
// primary 为空时取当前已启用 Profile 作为主卡；若该轮无其它可切换的 Profile，
// 仅刷新当前卡状态后返回。
func RunEuiccRotation(modemName string) error {
	m := GetModemService()
	conn, err := m.GetConn(modemName)
	if err != nil {
		return err
	}

	info, err := GetEuiccInfoViaLpa(conn)
	if err != nil {
		return err
	}

	cfg, _ := database.GetEuiccRotation(modemName)
	primary := cfg.PrimaryICCID
	if primary == "" {
		for _, p := range info.Profiles {
			if p.IsEnabled {
				primary = p.ICCID
				break
			}
		}
	}

	// 依次切换到非主卡收取短信
	for _, p := range info.Profiles {
		if p.ICCID == primary {
			continue
		}
		log.Printf("[%s] euicc rotation: switching to %s to collect sms", modemName, p.ICCID)
		if err := EnableProfileViaLpa(conn, p.ICCID); err != nil {
			log.Printf("[%s] euicc rotation: enable %s failed: %v", modemName, p.ICCID, err)
			continue
		}
		if !WaitForRegistration(conn, rotationRegWait) {
			log.Printf("[%s] euicc rotation: %s did not register in time", modemName, p.ICCID)
		}
		time.Sleep(rotationSmsSettle)
		if err := m.pollModemSms(modemName); err != nil {
			log.Printf("[%s] euicc rotation: poll sms for %s failed: %v", modemName, p.ICCID, err)
		}
	}

	// 收尾：切回主卡并等待驻网，恢复常态
	if primary != "" {
		if err := EnableProfileViaLpa(conn, primary); err != nil {
			log.Printf("[%s] euicc rotation: restore primary %s failed: %v", modemName, primary, err)
		} else {
			WaitForRegistration(conn, rotationRegWait)
			m.pollModemSms(modemName)
		}
	}

	_ = database.TouchEuiccRotationLastRun(modemName, time.Now())
	return nil
}

// StartEuiccRotationWatch 启动轮询调度：每隔 rotationTickInterval 检查所有已启用
// 轮询的设备，距上次轮询达到配置间隔则执行一轮。全局同时只允许一轮在跑，
// 避免多设备/多轮交叠导致模块被频繁切换。
func StartEuiccRotationWatch() {
	go func() {
		ticker := time.NewTicker(rotationTickInterval)
		defer ticker.Stop()
		// running 在调度协程与轮询协程间共享，需原子访问
		var running atomic.Bool
		for range ticker.C {
			if running.Load() {
				continue
			}
			rows, err := database.GetAllEuiccRotations()
			if err != nil {
				log.Printf("euicc rotation: list configs failed: %v", err)
				continue
			}
			now := time.Now()
			for _, row := range rows {
				if !row.Enabled || row.IntervalMin < RotationMinIntervalMin {
					continue
				}
				if !row.LastRunAt.IsZero() && now.Sub(row.LastRunAt) < time.Duration(row.IntervalMin)*time.Minute {
					continue
				}
				running.Store(true)
				go func(name string) {
					defer running.Store(false)
					log.Printf("[%s] euicc rotation: start cycle", name)
					if err := RunEuiccRotation(name); err != nil {
						log.Printf("[%s] euicc rotation: cycle failed: %v", name, err)
					}
					log.Printf("[%s] euicc rotation: cycle done", name)
				}(row.ModemName)
				break // 本 tick 只启动一轮，避免一次性排队多个设备
			}
		}
	}()
}
