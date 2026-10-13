package database

import (
	"time"

	"github.com/rehiy/web-modem/models"
)

// UpsertProfileNumber 按 ICCID 保存用户确认的手机号（MSISDN）。
// 自动读取的本机号码不走此方法，避免切卡期间的旧号码污染登记记录。
func UpsertProfileNumber(iccid, number string) error {
	if iccid == "" || number == "" {
		return nil
	}
	row := models.SimProfile{ICCID: iccid, Number: number, Verified: true, UpdatedAt: time.Now()}
	return db.Save(&row).Error
}

// GetProfileNumbers 批量查询已确认的 ICCID -> 手机号映射。
// 旧版本自动缓存的未确认记录会被忽略，避免继续展示错误号码。
func GetProfileNumbers(iccids []string) map[string]string {
	result := make(map[string]string, len(iccids))
	if len(iccids) == 0 {
		return result
	}
	var rows []models.SimProfile
	if err := db.Where("iccid IN ? AND verified = ?", iccids, true).Find(&rows).Error; err != nil {
		return result
	}
	for _, r := range rows {
		result[r.ICCID] = r.Number
	}
	return result
}

// DeleteProfileNumber 删除指定 ICCID 的号码缓存（删除 Profile 时同步清理）。
func DeleteProfileNumber(iccid string) error {
	if iccid == "" {
		return nil
	}
	return db.Where("iccid = ?", iccid).Delete(&models.SimProfile{}).Error
}
