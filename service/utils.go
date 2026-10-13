package service

import (
	"strings"
	"time"

	"github.com/rehiy/modem/at"
	"github.com/rehiy/web-modem/database"
	"github.com/rehiy/web-modem/models"
)

// atSmsToModelSms 将AT短信转换为数据库模型
func atSmsToModelSms(atSms at.Sms, conn *ModemConn) *models.Sms {
	sms := &models.Sms{
		Content:       atSms.Text,
		SmsIDs:        database.IntArrayToString(atSms.Indices),
		ReceiveTime:   parseSmsTime(atSms.Time),
		ReceiveNumber: connValue(conn, func(c *ModemConn) string { return c.State().Number }),
		SendNumber:    atSms.Number,
		Direction:     "in",
		ModemName:     connValue(conn, func(c *ModemConn) string { return c.Name }),
		DeviceIMEI:    connValue(conn, func(c *ModemConn) string { return c.State().IMEI }),
	}
	EnrichSmsCardInfo(sms, conn)
	return sms
}

func connValue(conn *ModemConn, getter func(*ModemConn) string) string {
	if conn == nil {
		return ""
	}
	return getter(conn)
}

// EnrichSmsCardInfo 为短信补充 SIM 和运营商信息，供查询与 Webhook 条件使用。
func EnrichSmsCardInfo(sms *models.Sms, conn *ModemConn) {
	if sms == nil || conn == nil {
		return
	}

	state := conn.State()
	if sms.DeviceIMEI == "" {
		sms.DeviceIMEI = state.IMEI
	}
	if sms.ModemName == "" {
		sms.ModemName = strings.TrimSpace(conn.Name)
	}
	if sms.ReceiveNumber == "" && sms.Direction == "in" {
		sms.ReceiveNumber = state.Number
	}
	if sms.SendNumber == "" && sms.Direction == "out" {
		sms.SendNumber = state.Number
	}

	if sms.SimICCID == "" {
		sms.SimICCID = state.ICCID
	}
	if sms.SimICCID == "" {
		if iccid := readConnICCID(conn); iccid != "" {
			conn.SetICCID(iccid)
			sms.SimICCID = iccid
		}
	}

	if sms.SimIMSI == "" {
		sms.SimIMSI = state.IMSI
	}
	if sms.SimIMSI == "" {
		if imsi, err := conn.GetIMSI(); err == nil {
			conn.SetIMSI(imsi)
			sms.SimIMSI = strings.TrimSpace(imsi)
		}
	}

	if sms.Operator == "" {
		sms.Operator = state.Operator
	}
	if sms.Operator == "" {
		if _, _, operator, _, err := conn.GetOperator(); err == nil {
			conn.SetOperator(operator)
			sms.Operator = strings.TrimSpace(operator)
		}
	}
}

func readConnICCID(conn *ModemConn) string {
	if iccid, err := conn.GetICCID(); err == nil {
		if normalized := normalizeCardIdentifier(iccid); normalized != "" {
			return normalized
		}
	}

	for _, command := range []string{"AT+QCCID", "AT+ICCID?", "AT+CCID?"} {
		responses, err := conn.SendCommand(command)
		if err != nil {
			continue
		}
		if iccid := parseICCIDResponses(responses); iccid != "" {
			return iccid
		}
	}
	return ""
}

func parseICCIDResponses(responses []string) string {
	for _, response := range responses {
		line := strings.TrimSpace(response)
		if line == "" || strings.EqualFold(line, "OK") {
			continue
		}
		if strings.Contains(line, ":") {
			parts := strings.SplitN(line, ":", 2)
			line = parts[1]
		}
		for _, part := range strings.FieldsFunc(line, func(r rune) bool {
			return r == ',' || r == ' ' || r == '\t' || r == '\r' || r == '\n'
		}) {
			if normalized := normalizeCardIdentifier(part); normalized != "" {
				return normalized
			}
		}
	}
	return ""
}

func normalizeCardIdentifier(value string) string {
	value = strings.Trim(strings.TrimSpace(value), `"`)
	if len(value) < 10 {
		return ""
	}
	for _, r := range value {
		if r < '0' || r > '9' {
			return ""
		}
	}
	return value
}

// parseSmsTime 解析短信时间字符串
func parseSmsTime(timeStr string) time.Time {
	if timeStr == "" {
		return time.Now()
	}

	// 尝试解析常见的短信时间格式
	formats := []string{
		"2006/01/02 15:04:05",
		"2006-01-02 15:04:05",
		"02/01/06 15:04:05",
	}

	for _, format := range formats {
		if t, err := time.Parse(format, timeStr); err == nil {
			return t
		}
	}

	// 如果无法解析，返回当前时间
	return time.Now()
}
