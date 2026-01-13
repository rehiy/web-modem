package service

import (
	"fmt"
	"log"

	"github.com/rehiy/web-modem/database"
	"github.com/rehiy/web-modem/models"
)

// SmsdbService 短信数据库服务
type SmsdbService struct {
	modemService *ModemService
}

// NewSmsdbService 创建短信数据库服务
func NewSmsdbService() *SmsdbService {
	return &SmsdbService{
		modemService: GetModemService(),
	}
}

// SyncSMSToDB 从指定Modem同步所有短信到数据库
func (s *SmsdbService) SyncSMSToDB(modemName string) (map[string]any, error) {
	// 获取连接
	conn, err := s.modemService.GetConn(modemName)
	if err != nil {
		return nil, fmt.Errorf("获取连接失败: %v", err)
	}

	// 列出所有短信（stat=4 表示所有短信）
	smsList, err := conn.ListSMSPdu(4)
	if err != nil {
		return nil, fmt.Errorf("读取短信失败: %v", err)
	}

	totalCount := len(smsList)
	newCount := 0

	// 同步每条短信
	for _, atSMS := range smsList {
		// 转换为数据库模型
		modelSMS := atSMSToModelSMS(atSMS, conn.Number, modemName)

		// 检查是否已存在
		if res, err := database.GetSMSListByIDs(atSMS.Indices); err == nil && len(res) > 0 {
			log.Printf("[%s] SMS already exists in database, skipping: %s", modemName, res[0].SMSIDs)
			continue
		}

		// 保存到数据库
		if err := database.CreateSMS(modelSMS); err != nil {
			log.Printf("[%s] Failed to save SMS to database: %v", modemName, err)
			continue
		}

		newCount++
		log.Printf("[%s] Synced SMS from %s to database: %s", modemName, atSMS.Number, atSMS.Text)
	}

	return map[string]any{
		"modemName":  modemName,
		"totalCount": totalCount,
		"newCount":   newCount,
	}, nil
}

// HandleIncomingSMS 处理接收到的短信：保存到数据库
func (w *SmsdbService) HandleIncomingSMS(dbSMS *models.SMS) {
	go func() {
		defer func() {
			if r := recover(); r != nil {
				log.Printf("[Webhook] Panic recovered: %v", r)
			}
		}()
		if database.IsSmsdbEnabled() {
			if err := database.CreateSMS(dbSMS); err != nil {
				log.Printf("[SMS] Failed to save incoming SMS: %v", err)
			}
		}
	}()
}
