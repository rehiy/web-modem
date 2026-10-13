package models

import "time"

// SimProfile 保存每个 eSIM Profile（按 ICCID 唯一）经用户确认的手机号（MSISDN）。
//
// 模块返回的本机号码可能为空或在切卡复位期间仍是旧卡数据，因此不将其自动写入。
// 只有用户在界面中明确保存的号码会标记为 Verified 并展示；ICCID 全局唯一，作为主键。
type SimProfile struct {
	ICCID     string    `json:"iccid" gorm:"primaryKey;column:iccid"`
	Number    string    `json:"number" gorm:"type:text"`
	Verified  bool      `json:"verified" gorm:"not null;default:false"`
	UpdatedAt time.Time `json:"updated_at"`
}
