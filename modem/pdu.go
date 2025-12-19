package modem

import (
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/warthog618/sms/encoding/pdumode"
	"github.com/warthog618/sms/encoding/tpdu"
	"github.com/warthog618/sms/encoding/ucs2"
)

// decodePDU 解析 PDU 字符串并返回发送者、时间、消息内容以及长短信信息
func decodePDU(pduStr string) (string, string, string, int, int, int, error) {
	raw, err := hex.DecodeString(pduStr)
	if err != nil {
		return "", "", "", 0, 0, 0, err
	}

	p, err := pdumode.UnmarshalBinary(raw)
	if err != nil {
		return "", "", "", 0, 0, 0, err
	}

	t := &tpdu.TPDU{}
	err = t.UnmarshalBinary(p.TPDU)
	if err != nil {
		return "", "", "", 0, 0, 0, err
	}

	if t.SmsType() != tpdu.SmsDeliver {
		return "", "", "", 0, 0, 0, fmt.Errorf("unsupported pdu type: %v", t.SmsType())
	}

	sender := t.OA.Number()
	timestamp := t.SCTS.Time.Format("2006/01/02 15:04:05")

	alpha, _ := t.Alphabet()
	msgBytes, err := tpdu.DecodeUserData(t.UD, t.UDH, alpha)
	message := ""
	if err != nil {
		message = fmt.Sprintf("<decode error: %v>", err)
	} else {
		message = string(msgBytes)
	}

	total, seq, ref, ok := t.ConcatInfo()
	if !ok {
		ref, total, seq = 0, 1, 1
	}

	return sender, timestamp, message, ref, total, seq, nil
}

// DecodeUCS2Hex decodes a hex string containing UCS2 data
func DecodeUCS2Hex(s string) string {
	s = strings.TrimSpace(s)
	b, err := hex.DecodeString(s)
	if err != nil {
		return s
	}
	runes, err := ucs2.Decode(b)
	if err != nil {
		return s
	}
	return string(runes)
}

// EncodeUCS2 encodes a string to UCS2 hex string
func EncodeUCS2(s string) string {
	return hex.EncodeToString(ucs2.Encode([]rune(s)))
}

// encodePDU encodes a message to PDU format for sending
func encodePDU(number, message string) (string, int, error) {
	// 创建一个新的 Submit PDU
	t, err := tpdu.NewSubmit()
	if err != nil {
		return "", 0, err
	}

	// 设置目标地址
	t.DA.SetNumber(number)

	// 设置编码为 UCS2 (支持中文和特殊字符)
	t.SetDCS(byte(tpdu.DcsUCS2Data))

	// 设置消息内容
	t.SetUD(ucs2.Encode([]rune(message)))

	// 创建完整的 PDU (包含 SCA)
	// 默认 SCA 为空 (00)，Modem 会使用 SIM 卡默认的短信中心
	tb, err := t.MarshalBinary()
	if err != nil {
		return "", 0, err
	}

	p := &pdumode.PDU{
		TPDU: tb,
	}

	// 序列化整个 PDU
	b, err := p.MarshalBinary()
	if err != nil {
		return "", 0, err
	}

	// 计算 TPDU 长度 (用于 AT+CMGS)
	// AT+CMGS 需要的是 TPDU 的长度（不包含 SCA）
	return strings.ToUpper(hex.EncodeToString(b)), len(tb), nil
}
