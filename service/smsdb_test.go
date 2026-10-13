package service

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/rehiy/modem/at"
	"github.com/rehiy/web-modem/database"
	"github.com/rehiy/web-modem/models"
)

func initTestDB(t *testing.T) {
	t.Helper()
	t.Setenv("DB_PATH", filepath.Join(t.TempDir(), "test.db"))
	if err := database.InitDB(); err != nil {
		t.Fatalf("init test db: %v", err)
	}
}

// TestFindExistingDeviceSms 覆盖设备短信查重：
// 已入库的短信应被识别为存在；设备删除短信后索引会被复用，
// 复用索引但内容/时间不同的新短信不能被误判为已存在（否则会被静默丢弃）。
func TestFindExistingDeviceSms(t *testing.T) {
	initTestDB(t)

	receiveTime := time.Date(2026, 7, 14, 10, 30, 0, 0, time.UTC)
	existing := &models.Sms{
		Content:       "old message",
		SmsIDs:        database.IntArrayToString([]int{5}),
		ReceiveTime:   receiveTime,
		ReceiveNumber: "+8613800138000",
		SendNumber:    "+8613800138001",
		Direction:     "in",
		ModemName:     "ttyUSB2",
		DeviceIMEI:    "123456789012345",
	}
	if err := database.CreateSms(existing); err != nil {
		t.Fatalf("create sms: %v", err)
	}

	atSms := at.Sms{Indices: []int{5}, Number: existing.SendNumber}

	// 同索引、同内容、同时间：应识别为已存在
	same := &models.Sms{
		Content:       existing.Content,
		ReceiveTime:   receiveTime,
		ReceiveNumber: existing.ReceiveNumber,
		SendNumber:    existing.SendNumber,
		ModemName:     existing.ModemName,
		DeviceIMEI:    existing.DeviceIMEI,
	}
	exists, err := FindExistingDeviceSms(atSms, same)
	if err != nil {
		t.Fatalf("find existing sms: %v", err)
	}
	if !exists {
		t.Fatalf("expected duplicate sms to be detected as existing")
	}

	// 索引被复用的新短信（内容与时间不同）：不能误判为已存在
	reused := &models.Sms{
		Content:       "new message on reused index",
		ReceiveTime:   receiveTime.Add(48 * time.Hour),
		ReceiveNumber: existing.ReceiveNumber,
		SendNumber:    existing.SendNumber,
		ModemName:     existing.ModemName,
		DeviceIMEI:    existing.DeviceIMEI,
	}
	exists, err = FindExistingDeviceSms(atSms, reused)
	if err != nil {
		t.Fatalf("find reused-index sms: %v", err)
	}
	if exists {
		t.Fatalf("sms on reused index must not be treated as existing")
	}
}
