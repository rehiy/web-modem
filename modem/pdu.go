package modem

import (
	"encoding/hex"
	"fmt"
	"strings"
	"unicode/utf16"
)

// decodePDU 解析 PDU 字符串并返回发送者、时间、消息内容以及长短信信息
func decodePDU(pdu string) (string, string, string, int, int, int, error) {
	data, err := hex.DecodeString(pdu)
	if err != nil {
		return "", "", "", 0, 0, 0, err
	}

	offset := 0

	// 1. Service Center Address (SCA)
	if offset >= len(data) {
		return "", "", "", 0, 0, 0, fmt.Errorf("short pdu: sca")
	}
	scaLen := int(data[offset])
	offset += 1 + scaLen

	// 2. PDU Type
	if offset >= len(data) {
		return "", "", "", 0, 0, 0, fmt.Errorf("short pdu: type")
	}
	pduType := data[offset]
	hasUDH := (pduType & 0x40) != 0
	offset++

	// 3. Originating Address (OA)
	if offset >= len(data) {
		return "", "", "", 0, 0, 0, fmt.Errorf("short pdu: oa len")
	}
	oaLen := int(data[offset]) // Length in digits (or septets for alphanumeric)
	offset++
	if offset >= len(data) {
		return "", "", "", 0, 0, 0, fmt.Errorf("short pdu: oa type")
	}
	oaType := data[offset]
	offset++

	oaByteLen := (oaLen + 1) / 2

	if offset+oaByteLen > len(data) {
		return "", "", "", 0, 0, 0, fmt.Errorf("short pdu: oa data")
	}
	oaBytes := data[offset : offset+oaByteLen]
	offset += oaByteLen

	sender := decodeAddress(oaBytes, oaType, oaLen)

	// 4. Protocol Identifier (PID)
	offset++

	// 5. Data Coding Scheme (DCS)
	if offset >= len(data) {
		return "", "", "", 0, 0, 0, fmt.Errorf("short pdu: dcs")
	}
	dcs := data[offset]
	offset++

	// 6. Service Centre Time Stamp (SCTS)
	if offset+7 > len(data) {
		return "", "", "", 0, 0, 0, fmt.Errorf("short pdu: scts")
	}
	timestamp := decodeTimestamp(data[offset : offset+7])
	offset += 7

	// 7. User Data Length (UDL)
	if offset >= len(data) {
		return "", "", "", 0, 0, 0, fmt.Errorf("short pdu: udl")
	}
	udl := int(data[offset])
	offset++

	// 8. User Data (UD)
	if offset > len(data) {
		return "", "", "", 0, 0, 0, fmt.Errorf("short pdu: ud")
	}
	
	ref, total, seq := 0, 1, 1
	if hasUDH {
		ref, total, seq = parseUDH(data[offset:])
	}
	
	message := decodeUserData(data[offset:], udl, dcs, hasUDH)

	return sender, timestamp, message, ref, total, seq, nil
}

func parseUDH(b []byte) (int, int, int) {
	if len(b) == 0 {
		return 0, 1, 1
	}
	udhLen := int(b[0])
	if udhLen > len(b)-1 {
		udhLen = len(b) - 1
	}
	
	// Iterate over IEs
	idx := 1
	end := 1 + udhLen
	
	for idx < end {
		if idx+1 >= end {
			break
		}
		iei := b[idx]
		iedl := int(b[idx+1])
		idx += 2
		
		if idx+iedl > end {
			break
		}
		
		if iei == 0x00 && iedl == 3 {
			// 8-bit reference
			return int(b[idx]), int(b[idx+1]), int(b[idx+2])
		} else if iei == 0x08 && iedl == 4 {
			// 16-bit reference
			ref := int(b[idx])<<8 | int(b[idx+1])
			return ref, int(b[idx+2]), int(b[idx+3])
		}
		
		idx += iedl
	}
	
	return 0, 1, 1
}

func decodeAddress(b []byte, addrType byte, length int) string {
	if addrType == 0xD0 { // Alphanumeric 7-bit
		return decode7Bit(b, length)
	}
	// Standard phone number
	s := swapSemiOctets(hex.EncodeToString(b))
	if length < len(s) {
		s = s[:length]
	}
	if addrType == 0x91 {
		return "+" + s
	}
	return s
}

func swapSemiOctets(s string) string {
	var sb strings.Builder
	for i := 0; i < len(s); i += 2 {
		if i+1 < len(s) {
			sb.WriteByte(s[i+1])
			sb.WriteByte(s[i])
		} else {
			sb.WriteByte(s[i])
		}
	}
	return strings.TrimSuffix(sb.String(), "F")
}

func decodeTimestamp(b []byte) string {
	s := swapSemiOctets(hex.EncodeToString(b))
	if len(s) < 12 {
		return ""
	}
	// YY/MM/DD,HH:MM:SS
	return fmt.Sprintf("20%s/%s/%s %s:%s:%s", s[0:2], s[2:4], s[4:6], s[6:8], s[8:10], s[10:12])
}

func decodeUserData(b []byte, length int, dcs byte, hasUDH bool) string {
	headerLen := 0
	if hasUDH {
		if len(b) == 0 {
			return ""
		}
		udhLen := int(b[0])
		headerLen = 1 + udhLen
		if headerLen > len(b) {
			headerLen = len(b)
		}
	}

	coding := (dcs >> 2) & 0x03
	if coding == 2 { // UCS2
		data := b
		if headerLen > 0 {
			data = b[headerLen:]
			length -= headerLen // UCS2 length is in bytes
		}
		if length > len(data) {
			length = len(data)
		}
		return decodeUCS2(data[:length])
	}

	// Default to 7-bit
	// For 7-bit, length is in septets.
	// If UDH is present, we decode the whole thing and skip the header septets.
	byteLen := (length*7 + 7) / 8
	if byteLen > len(b) {
		byteLen = len(b)
	}
	
	fullMsg := decode7Bit(b[:byteLen], length)
	
	if hasUDH {
		// Calculate how many septets the header + padding occupies
		// Header bits = headerLen * 8
		// Septets = ceil(Header bits / 7)
		septetsToSkip := (headerLen*8 + 6) / 7
		if septetsToSkip < len(fullMsg) {
			return fullMsg[septetsToSkip:]
		}
		return ""
	}
	
	return fullMsg
}

func decodeUCS2(b []byte) string {
	u16 := make([]uint16, len(b)/2)
	for i := range u16 {
		u16[i] = uint16(b[i*2])<<8 | uint16(b[i*2+1])
	}
	return string(utf16.Decode(u16))
}

// DecodeUCS2Hex decodes a hex string containing UCS2 data
func DecodeUCS2Hex(s string) string {
	s = strings.TrimSpace(s)
	b, err := hex.DecodeString(s)
	if err != nil || len(b)%2 != 0 {
		return s
	}
	return decodeUCS2(b)
}

// EncodeUCS2 encodes a string to UCS2 hex string
func EncodeUCS2(s string) string {
	u16 := utf16.Encode([]rune(s))
	var sb strings.Builder
	for _, r := range u16 {
		sb.WriteString(fmt.Sprintf("%04X", r))
	}
	return sb.String()
}

// encodePDU encodes a message to PDU format for sending
func encodePDU(number, message string) (string, int, error) {
	// 1. SCA: 00 (Use default from SIM)
	sca := "00"

	// 2. PDU Type: 01 (SMS-SUBMIT)
	pduType := "01"

	// 3. TP-MR: 00
	mr := "00"

	// 4. TP-DA
	da := encodeAddressField(number)

	// 5. TP-PID: 00
	pid := "00"

	// 6. TP-DCS: 08 (UCS2)
	dcs := "08"

	// 7. TP-VP: Not present (since VPF=00 in PDU Type 01)

	// 8. TP-UDL & TP-UD
	ud := EncodeUCS2(message)
	udBytes, _ := hex.DecodeString(ud)
	udl := fmt.Sprintf("%02X", len(udBytes))

	pdu := sca + pduType + mr + da + pid + dcs + udl + ud

	// Calculate length for AT+CMGS (TPDU length in octets)
	// Total PDU bytes - SCA bytes (1 byte for "00")
	cmgsLen := (len(pdu) / 2) - 1

	return strings.ToUpper(pdu), cmgsLen, nil
}

func encodeAddressField(number string) string {
	number = strings.TrimPrefix(number, "+")
	length := len(number)
	toa := "91" // International format

	// Pad with F if odd length
	paddedNumber := number
	if length%2 != 0 {
		paddedNumber += "F"
	}

	swapped := swapBytes(paddedNumber)
	return fmt.Sprintf("%02X", length) + toa + swapped
}

func swapBytes(s string) string {
	var sb strings.Builder
	for i := 0; i < len(s); i += 2 {
		if i+1 < len(s) {
			sb.WriteByte(s[i+1])
			sb.WriteByte(s[i])
		} else {
			sb.WriteByte(s[i])
		}
	}
	return sb.String()
}

func decode7Bit(b []byte, numSeptets int) string {
	var res []byte
	var shift byte = 0
	var prev byte = 0

	for _, v := range b {
		current := (v << shift) | prev
		prev = v >> (7 - shift)
		res = append(res, current&0x7F)
		shift++

		if shift == 7 {
			res = append(res, prev)
			shift = 0
			prev = 0
		}
	}
	if len(res) > numSeptets {
		res = res[:numSeptets]
	}
	return string(res)
}
