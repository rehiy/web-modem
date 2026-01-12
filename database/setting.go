package database

import (
	"fmt"

	"github.com/rehiy/web-modem/models"
)

// GetSettings 获取所有设置
func GetSettings() (map[string]string, error) {
	var settings []models.Setting
	if err := db.Find(&settings).Error; err != nil {
		return nil, fmt.Errorf("failed to get settings: %w", err)
	}

	result := make(map[string]string)
	for _, setting := range settings {
		result[setting.Key] = setting.Value
	}

	return result, nil
}

// IsSmsdbEnabled 检查短信存储是否启用
func IsSmsdbEnabled() bool {
	var setting models.Setting
	result := db.Where("key = ?", "smsdb_enabled").First(&setting)
	if result.Error != nil {
		return false
	}
	return setting.Value == "true"
}

// SetSmsdbEnabled 设置短信存储启用状态
func SetSmsdbEnabled(enabled bool) error {
	value := "false"
	if enabled {
		value = "true"
	}

	setting := models.Setting{Key: "smsdb_enabled", Value: value}
	err := db.Where(models.Setting{Key: "smsdb_enabled"}).Assign(setting).FirstOrCreate(&setting).Error
	if err != nil {
		return fmt.Errorf("failed to set smsdb_enabled: %w", err)
	}
	return nil
}

// IsWebhookEnabled 检查webhook功能是否启用
func IsWebhookEnabled() bool {
	var setting models.Setting
	result := db.Where("key = ?", "webhook_enabled").First(&setting)
	if result.Error != nil {
		return false
	}
	return setting.Value == "true"
}

// SetWebhookEnabled 设置webhook功能启用状态
func SetWebhookEnabled(enabled bool) error {
	value := "false"
	if enabled {
		value = "true"
	}

	setting := models.Setting{Key: "webhook_enabled", Value: value}
	result := db.Where(models.Setting{Key: "webhook_enabled"}).Assign(setting).FirstOrCreate(&setting)
	if result.Error != nil {
		return fmt.Errorf("failed to set webhook_enabled: %w", result.Error)
	}
	return nil
}

// InitDefaultSettings 初始化默认设置
func InitDefaultSettings() error {
	defaultSettings := map[string]string{
		"smsdb_enabled":   "true",
		"webhook_enabled": "false",
	}

	for key, value := range defaultSettings {
		setting := models.Setting{Key: key, Value: value}
		result := db.FirstOrCreate(&setting, models.Setting{Key: key})
		if result.Error != nil {
			return fmt.Errorf("failed to insert default setting: %w", result.Error)
		}
	}

	return nil
}
