package database

import (
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"

	"github.com/rehiy/web-modem/models"
)

// CreateSMS 保存短信到数据库
func CreateSMS(sms *models.SMS) error {
	// 确保必要字段已设置
	if sms.Direction == "" {
		sms.Direction = "in"
	}
	if sms.ReceiveTime.IsZero() {
		sms.ReceiveTime = time.Now()
	}

	err := db.Create(sms).Error
	if err != nil {
		return fmt.Errorf("failed to save SMS: %w", err)
	}
	return nil
}

// DeleteSMS 根据数据库ID删除短信
func DeleteSMS(id int) error {
	ret := db.Delete(&models.SMS{}, id)
	if ret.Error != nil {
		return fmt.Errorf("failed to delete SMS: %w", ret.Error)
	}
	if ret.RowsAffected == 0 {
		return fmt.Errorf("SMS not found")
	}
	return nil
}

// BatchDeleteSMS 批量删除短信
func BatchDeleteSMS(ids []int) error {
	if len(ids) == 0 {
		return nil
	}

	err := db.Where("id IN ?", ids).Delete(&models.SMS{}).Error
	if err != nil {
		return fmt.Errorf("failed to batch delete SMS: %w", err)
	}
	return nil
}

// GetSMSListByIDs 根据短信模块的ID查询
func GetSMSListByIDs(smsIDs []int) ([]models.SMS, error) {
	if len(smsIDs) == 0 {
		return []models.SMS{}, nil
	}

	var smsList []models.SMS
	str := IntArrayToString(smsIDs)
	err := db.Where("sms_ids = ?", str).Order("receive_time DESC").Find(&smsList).Error
	if err != nil {
		return nil, fmt.Errorf("failed to query SMS by IDs: %w", err)
	}
	return smsList, nil
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
	if filter.ModemName != "" {
		query = query.Where("modem_name = ?", filter.ModemName)
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
	err := query.Order("receive_time DESC").Limit(filter.Limit).Offset(filter.Offset).Find(&smsList).Error
	if err != nil {
		return nil, 0, fmt.Errorf("failed to query SMS: %w", err)
	}

	return smsList, int(total), nil
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
