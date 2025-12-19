package modem

import (
	"encoding/hex"
	"strings"
	"time"

	"github.com/xlab/at/pdu"
	"github.com/xlab/at/sms"
)

// decodePDU 解析 PDU 字符串并返回发送者、时间、消息内容以及长短信信息
func decodePDU(pduStr string) (string, string, string, int, int, int, error) {
	raw, err := hex.DecodeString(strings.TrimSpace(pduStr))
	if err != nil {
		return "", "", "", 0, 0, 0, err
	}

	var msg sms.Message
	if _, err := msg.ReadFrom(raw); err != nil {
		return "", "", "", 0, 0, 0, err
	}

	sender := string(msg.Address)
	timestamp := time.Time(msg.ServiceCenterTime).Format("2006/01/02 15:04:05")
	total, seq, ref := 1, 1, 0
	if msg.UserDataHeader.TotalNumber > 0 && msg.UserDataHeader.Sequence > 0 {
		total = msg.UserDataHeader.TotalNumber
		seq = msg.UserDataHeader.Sequence
		ref = msg.UserDataHeader.Tag
	}

	return sender, timestamp, msg.Text, ref, total, seq, nil
}

// DecodeUCS2Hex decodes a hex string containing UCS2 data
func DecodeUCS2Hex(s string) string {
	b, err := hex.DecodeString(strings.TrimSpace(s))
	if err != nil {
		return s
	}
	decoded, err := pdu.DecodeUcs2(b, false)
	if err != nil {
		return s
	}
	return decoded
}

// encodePDU encodes a message to PDU format for sending
func encodePDU(number, message string) (string, int, error) {
	msg := sms.Message{
		Type:    sms.MessageTypes.Submit,
		Address: sms.PhoneNumber(number),
		Text:    message,
	}

	if pdu.Is7BitEncodable(message) {
		msg.Encoding = sms.Encodings.Gsm7Bit
	} else {
		msg.Encoding = sms.Encodings.UCS2
	}

	tpduLen, octets, err := msg.PDU()
	if err != nil {
		return "", 0, err
	}

	return strings.ToUpper(hex.EncodeToString(octets)), tpduLen, nil
}
