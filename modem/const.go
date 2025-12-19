package modem

import (
	"errors"
	"regexp"
	"time"
)

const (
	// AT 命令
	cmdEchoOff      = "ATE0"
	cmdTextMode     = "AT+CMGF=0"
	cmdCharSet      = "AT+CSCS=\"GSM\""
	cmdCheck        = "AT"
	cmdListSMS      = "AT+CMGL=4"
	cmdDeleteSMS    = "AT+CMGD=%d"
	cmdSendSMS      = "AT+CMGS=%d"
	cmdSignal       = "AT+CSQ"
	cmdManufacturer = "AT+CGMI"
	cmdModel        = "AT+CGMM"
	cmdIMEI         = "AT+CGSN"
	cmdIMSI         = "AT+CIMI"
	cmdOperator     = "AT+COPS?"
	cmdNumber       = "AT+CNUM"

	// 特殊字符
	eof      = "\r\n"
	ctrlZ    = "\x1A"

	// 常用响应
	respOK     = "OK"
	respError  = "ERROR"

	// 延迟和超时
	bufferSize      = 128
	readTimeout     = 100 * time.Millisecond
	errorSleep      = 100 * time.Millisecond
)

var (
	// 常用错误
	errNotFound   = errors.New("not found")
	errTimeout    = errors.New("command timeout")

	// 正则表达式
	rePhoneNumber = regexp.MustCompile(`\+CNUM:.*,"([^"]+)"`)
	reOperator    = regexp.MustCompile(`"([^"]+)"`)
)
