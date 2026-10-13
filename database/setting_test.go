package database

import (
	"path/filepath"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/rehiy/web-modem/models"
	"gorm.io/gorm"
)

func TestSettingsRespectStoredSwitches(t *testing.T) {
	testDB, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "settings.db")), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	previous := db
	db = testDB
	t.Cleanup(func() {
		_ = Close()
		db = previous
	})
	if err := db.AutoMigrate(&models.Setting{}); err != nil {
		t.Fatal(err)
	}
	if err := InitDefaultSettings(); err != nil {
		t.Fatal(err)
	}
	if !IsSmsdbEnabled() || IsWebhookEnabled() {
		t.Fatal("默认应启用短信库、禁用 Webhook")
	}
	for _, enabled := range []bool{false, true, false} {
		if err := SetSmsdbEnabled(enabled); err != nil {
			t.Fatal(err)
		}
		if err := SetWebhookEnabled(enabled); err != nil {
			t.Fatal(err)
		}
		if err := InitDefaultSettings(); err != nil {
			t.Fatal(err)
		}
		if IsSmsdbEnabled() != enabled || IsWebhookEnabled() != enabled {
			t.Fatalf("开关或重新初始化未保留 enabled=%v", enabled)
		}
		settings, err := GetSettings()
		if err != nil {
			t.Fatal(err)
		}
		for _, key := range []string{"smsdb_enabled", "webhook_enabled"} {
			if (settings[key] == "true") != enabled {
				t.Fatalf("API 设置 %s=%q 与存储开关不一致", key, settings[key])
			}
		}
	}
}
