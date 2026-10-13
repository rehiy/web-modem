package database

import (
	"errors"
	"fmt"

	"gorm.io/gorm"

	"github.com/rehiy/web-modem/models"
)

// GetEuiccRotation 读取指定设备的轮询配置，不存在则返回零值结构（不报错）。
func GetEuiccRotation(modemName string) (models.EuiccRotation, error) {
	var row models.EuiccRotation
	err := db.Where("modem_name = ?", modemName).First(&row).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return models.EuiccRotation{ModemName: modemName}, nil
		}
		return models.EuiccRotation{ModemName: modemName}, fmt.Errorf("failed to get euicc rotation: %w", err)
	}
	return row, nil
}

// GetAllEuiccRotations 返回所有已配置的轮询记录（含未启用的）。
func GetAllEuiccRotations() ([]models.EuiccRotation, error) {
	var rows []models.EuiccRotation
	if err := db.Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("failed to list euicc rotations: %w", err)
	}
	return rows, nil
}

// SaveEuiccRotation 保存（新增或更新）指定设备的轮询配置。
func SaveEuiccRotation(row models.EuiccRotation) error {
	if row.ModemName == "" {
		return fmt.Errorf("modem_name is required")
	}
	return db.Save(&row).Error
}

// TouchEuiccRotationLastRun 更新指定设备的上次轮询结束时间。
func TouchEuiccRotationLastRun(modemName string, lastRunAt interface{}) error {
	return db.Model(&models.EuiccRotation{}).
		Where("modem_name = ?", modemName).
		Update("last_run_at", lastRunAt).Error
}
