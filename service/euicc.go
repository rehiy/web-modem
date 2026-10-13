package service

import (
	"encoding/hex"
	"fmt"
	"strings"
	"time"
)

// EuiccProfile represents a single eSIM profile.
type EuiccProfile struct {
	ICCID               string `json:"iccid"`
	Number              string `json:"number,omitempty"`
	Nickname            string `json:"nickname,omitempty"`
	ServiceProviderName string `json:"serviceProviderName,omitempty"`
	ProfileName         string `json:"profileName,omitempty"`
	State               string `json:"state"`
	IsEnabled           bool   `json:"isEnabled"`
}

// EuiccInfo contains the EID and the full profile list from an eUICC.
type EuiccInfo struct {
	EID      string         `json:"eid"`
	Profiles []EuiccProfile `json:"profiles"`
}

// splitAPDUResponse 把 +CSIM 返回的十六进制串拆为数据部分与 SW1SW2。
// 供 csimChannel（euicc-go 适配器）解析 MANAGE CHANNEL / SELECT 响应使用。
func splitAPDUResponse(resp string) (data []byte, sw string, err error) {
	if len(resp) < 4 {
		return nil, "", fmt.Errorf("APDU response too short: %q", resp)
	}
	sw = resp[len(resp)-4:]
	if dataHex := resp[:len(resp)-4]; dataHex != "" {
		data, err = hex.DecodeString(dataHex)
		if err != nil {
			return nil, "", fmt.Errorf("decode APDU data: %w", err)
		}
	}
	return data, sw, nil
}

// WaitForNetworkReady waits up to maxWait for the SIM to become ready after a profile switch.
func WaitForNetworkReady(conn *ModemConn, maxWait time.Duration) {
	deadline := time.Now().Add(maxWait)
	for time.Now().Before(deadline) {
		status, err := conn.GetSIMStatus()
		if err == nil && strings.EqualFold(strings.TrimSpace(status), "READY") {
			return
		}
		time.Sleep(500 * time.Millisecond)
	}
}
