package database

import (
	"fmt"

	"gorm.io/gorm"

	"github.com/rehiy/web-modem/models"
)

// CreateWebhook 创建webhook配置
func CreateWebhook(webhook *models.Webhook) error {
	result := db.Create(webhook)
	if result.Error != nil {
		return fmt.Errorf("failed to create webhook: %w", result.Error)
	}
	return nil
}

// UpdateWebhook 更新webhook配置
func UpdateWebhook(webhook *models.Webhook) error {
	result := db.Save(webhook)
	if result.Error != nil {
		return fmt.Errorf("failed to update webhook: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("webhook not found")
	}
	return nil
}

// DeleteWebhook 删除webhook配置
func DeleteWebhook(id int) error {
	result := db.Delete(&models.Webhook{}, id)
	if result.Error != nil {
		return fmt.Errorf("failed to delete webhook: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("webhook not found")
	}
	return nil
}

// GetWebhook 根据ID获取webhook配置
func GetWebhook(id int) (*models.Webhook, error) {
	var webhook models.Webhook
	result := db.First(&webhook, id)
	if result.Error != nil {
		if result.Error == gorm.ErrRecordNotFound {
			return nil, fmt.Errorf("webhook not found")
		}
		return nil, fmt.Errorf("failed to get webhook: %w", result.Error)
	}
	return &webhook, nil
}

// GetWebhookList 获取所有webhook配置
func GetWebhookList() ([]models.Webhook, error) {
	var webhooks []models.Webhook
	result := db.Order("created_at DESC").Find(&webhooks)
	if result.Error != nil {
		return nil, fmt.Errorf("failed to query webhooks: %w", result.Error)
	}
	return webhooks, nil
}

// GetEnabledWebhookList 获取所有启用的webhook
func GetEnabledWebhookList() ([]models.Webhook, error) {
	var webhooks []models.Webhook
	result := db.Where("enabled = ?", true).Order("created_at DESC").Find(&webhooks)
	if result.Error != nil {
		return nil, fmt.Errorf("failed to query enabled webhooks: %w", result.Error)
	}
	return webhooks, nil
}
