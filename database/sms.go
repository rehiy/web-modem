package database

import (
	"fmt"
	"strings"
	"time"

	"github.com/rehiy/web-modem/models"
	"gorm.io/gorm"
)

// SaveSMS 保存短信到数据库
func SaveSMS(sms *models.SMS) error {
	// 确保必要字段已设置
	if sms.ReceiveTime.IsZero() {
		sms.ReceiveTime = time.Now()
	}
	if sms.Direction == "" {
		sms.Direction = "in"
	}

	result := db.Create(sms)
	if result.Error != nil {
		return fmt.Errorf("failed to save SMS: %w", result.Error)
	}
	return nil
}

// GetSMSList 查询短信列表
func GetSMSList(filter *models.SMSFilter) ([]models.SMS, int, error) {
	query := db.Model(&models.SMS{})

	if filter.Direction != "" {
		query = query.Where("direction = ?", filter.Direction)
	}
	if filter.SendNumber != "" {
		query = query.Where("send_number = ?", filter.SendNumber)
	}
	if !filter.StartTime.IsZero() {
		query = query.Where("receive_time >= ?", filter.StartTime)
	}
	if !filter.EndTime.IsZero() {
		query = query.Where("receive_time <= ?", filter.EndTime)
	}

	// 查询总数
	var total int64
	countQuery := query.Session(&gorm.Session{})
	if err := countQuery.Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("failed to count SMS: %w", err)
	}

	// 查询列表
	var smsList []models.SMS
	err := query.Order("receive_time DESC").
		Limit(filter.Limit).
		Offset(filter.Offset).
		Find(&smsList).Error
	if err != nil {
		return nil, 0, fmt.Errorf("failed to query SMS: %w", err)
	}

	return smsList, int(total), nil
}

// DeleteSMSByID 根据数据库ID删除短信
func DeleteSMSByID(id int) error {
	result := db.Delete(&models.SMS{}, id)
	if result.Error != nil {
		return fmt.Errorf("failed to delete SMS: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("SMS not found")
	}
	return nil
}

// BatchDeleteSMS 批量删除短信
func BatchDeleteSMS(ids []int) error {
	if len(ids) == 0 {
		return nil
	}
	result := db.Where("id IN ?", ids).Delete(&models.SMS{})
	if result.Error != nil {
		return fmt.Errorf("failed to batch delete SMS: %w", result.Error)
	}
	return nil
}

// GetsmsdbBodyBySMSIDs 根据短信模块的ID查询
func GetsmsdbBodyBySMSIDs(smsIDs []int) ([]models.SMS, error) {
	if len(smsIDs) == 0 {
		return []models.SMS{}, nil
	}
	idStr := IntArrayToString(smsIDs)
	var smsList []models.SMS
	err := db.Where("sms_ids = ?", idStr).
		Order("receive_time DESC").
		Find(&smsList).Error
	if err != nil {
		return nil, fmt.Errorf("failed to query SMS by IDs: %w", err)
	}
	return smsList, nil
}

// IsSmsdbEnabled 检查是否启用了数据库存储短信
func IsSmsdbEnabled() bool {
	var setting models.Setting
	result := db.Where("key = ?", "smsdb_enabled").First(&setting)
	if result.Error != nil {
		return false
	}
	return setting.Value == "true"
}

// SaveIncomingSMS 保存接收到的短信
func SaveIncomingSMS(smsData *models.SMS) (*models.SMS, error) {
	if !IsSmsdbEnabled() {
		return nil, nil
	}

	smsData.Direction = "in"
	err := SaveSMS(smsData)
	if err != nil {
		return nil, err
	}
	return smsData, nil
}

// SaveOutgoingSMS 保存发送的短信
func SaveOutgoingSMS(smsData *models.SMS) (*models.SMS, error) {
	if !IsSmsdbEnabled() {
		return nil, nil
	}

	smsData.Direction = "out"
	err := SaveSMS(smsData)
	if err != nil {
		return nil, err
	}
	return smsData, nil
}

// IntArrayToString 将int数组转换为字符串
func IntArrayToString(arr []int) string {
	if len(arr) == 0 {
		return ""
	}
	strs := make([]string, len(arr))
	for i, v := range arr {
		strs[i] = fmt.Sprintf("%d", v)
	}
	return strings.Join(strs, ",")
}

// GetSettings 获取所有设置
func GetSettings() (map[string]string, error) {
	var settings []models.Setting
	err := db.Find(&settings).Error
	if err != nil {
		return nil, fmt.Errorf("failed to get settings: %w", err)
	}

	result := make(map[string]string)
	for _, setting := range settings {
		result[setting.Key] = setting.Value
	}
	return result, nil
}

// SetSmsdbEnabled 设置短信存储启用状态
func SetSmsdbEnabled(enabled bool) error {
	value := "false"
	if enabled {
		value = "true"
	}

	setting := models.Setting{Key: "smsdb_enabled", Value: value}
	result := db.Where(models.Setting{Key: "smsdb_enabled"}).Assign(setting).FirstOrCreate(&setting)
	if result.Error != nil {
		return fmt.Errorf("failed to set smsdb_enabled: %w", result.Error)
	}
	return nil
}
