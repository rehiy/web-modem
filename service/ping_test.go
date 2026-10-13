package service

import (
	"math"
	"testing"
)

func TestIsValidPingHost(t *testing.T) {
	valid := []string{"8.8.8.8", "2001:4860:4860::8888", "www.baidu.com", "example-host.com", "a"}
	for _, host := range valid {
		if !IsValidPingHost(host) {
			t.Errorf("host %q should be valid", host)
		}
	}

	invalid := []string{"-flag", "a;b", `a"b`, "a b", "host_name!", "", "a,b", "-c"}
	for _, host := range invalid {
		if IsValidPingHost(host) {
			t.Errorf("host %q should be invalid", host)
		}
	}
}

func TestNormalizePingTarget(t *testing.T) {
	host, count, err := normalizePingTarget("", 0)
	if err != nil || host != defaultPingHost || count != 4 {
		t.Errorf("empty target should default to %s/4, got %s/%d err=%v", defaultPingHost, host, count, err)
	}

	if _, count, _ := normalizePingTarget("8.8.8.8", 99); count != maxPingCount {
		t.Errorf("count should be clamped to %d, got %d", maxPingCount, count)
	}

	if _, _, err := normalizePingTarget("8.8.8.8; rm -rf", 4); err == nil {
		t.Error("injection-style host should be rejected")
	}
}

func TestApplyQuectelPingURC(t *testing.T) {
	result := &PingResult{}

	// 逐包应答
	applyPingURC("quectel", []string{"0", "8.8.8.8", "32", "161", "255"}, result)
	if len(result.Replies) != 1 || result.Replies[0].TimeMs != 161 || result.Replies[0].TTL != 255 {
		t.Fatalf("unexpected reply: %+v", result.Replies)
	}

	// 单包超时
	applyPingURC("quectel", []string{"1"}, result)
	if len(result.Replies) != 2 || !result.Replies[1].Timeout {
		t.Fatalf("expected timeout reply, got %+v", result.Replies)
	}

	// 最终统计
	applyPingURC("quectel", []string{"0", "4", "3", "1", "161", "170", "165"}, result)
	if !result.Finished || result.Sent != 4 || result.Received != 3 || result.Lost != 1 || result.AvgMs != 165 {
		t.Fatalf("unexpected summary: %+v", result)
	}
}

func TestApplyQuectelPingURCError(t *testing.T) {
	result := &PingResult{}
	applyPingURC("quectel", []string{"565"}, result)
	if !result.Finished || result.ErrorMsg == "" {
		t.Fatalf("error code should finish with error message: %+v", result)
	}
}

func TestApplyCmccPingURC(t *testing.T) {
	result := &PingResult{}

	applyPingURC("cmcc", []string{"0", "2409:8C54:870:34E:0:FF:B024:1916", "16", "60", "49"}, result)
	if len(result.Replies) != 1 || result.Replies[0].TimeMs != 60 {
		t.Fatalf("unexpected reply: %+v", result.Replies)
	}

	applyPingURC("cmcc", []string{"statistics", "4", "1", "60", "75", "67"}, result)
	if !result.Finished || result.Sent != 4 || result.Lost != 1 || result.Received != 3 || result.AvgMs != 67 {
		t.Fatalf("unexpected summary: %+v", result)
	}
}

func TestApplyCmccPingURCError(t *testing.T) {
	// 响应超时按单包丢失计，不结束会话
	result := &PingResult{}
	applyPingURC("cmcc", []string{"4"}, result)
	if result.Finished || len(result.Replies) != 1 || !result.Replies[0].Timeout {
		t.Fatalf("code 4 should append timeout reply: %+v", result)
	}

	// DNS 解析失败直接结束并带错误说明
	result = &PingResult{}
	applyPingURC("cmcc", []string{"1"}, result)
	if !result.Finished || result.ErrorMsg == "" {
		t.Fatalf("code 1 should finish with error message: %+v", result)
	}
}

func TestCmccPingCommandCount(t *testing.T) {
	// ML307 语法 AT+MPING=<host>[,<timeout>[,<ping_num>...]]，包数必须透传
	for _, variant := range pingVariants {
		if variant.key != "cmcc" {
			continue
		}
		if cmd := variant.build("8.8.8.8", 1); cmd != `AT+MPING="8.8.8.8",4,1` {
			t.Fatalf("unexpected cmcc command: %s", cmd)
		}
	}
}

func TestApplySimcomPingURC(t *testing.T) {
	result := &PingResult{}

	applyPingURC("simcom", []string{"1", "8.8.8.8", "64", "88", "116"}, result)
	applyPingURC("simcom", []string{"2"}, result)
	applyPingURC("simcom", []string{"3", "4", "3", "1", "80", "95", "88"}, result)

	if len(result.Replies) != 2 || !result.Replies[1].Timeout {
		t.Fatalf("unexpected replies: %+v", result.Replies)
	}
	if !result.Finished || result.Sent != 4 || result.Received != 3 || result.MaxMs != 95 {
		t.Fatalf("unexpected summary: %+v", result)
	}
}

func TestFinalizePingResultWithoutSummary(t *testing.T) {
	result := &PingResult{
		Replies: []PingReply{
			{TimeMs: 10},
			{TimeMs: 20},
			{Timeout: true},
		},
	}
	finalizePingResult(result)

	if result.Sent != 3 || result.Received != 2 || result.Lost != 1 {
		t.Fatalf("unexpected counters: %+v", result)
	}
	if result.MinMs != 10 || result.MaxMs != 20 || math.Abs(result.AvgMs-15) > 0.001 {
		t.Fatalf("unexpected rtt stats: %+v", result)
	}
}

func TestParseHostPingOutputLinux(t *testing.T) {
	output := `PING 8.8.8.8 (8.8.8.8) 56(84) bytes of data.
64 bytes from 8.8.8.8: icmp_seq=1 ttl=115 time=9.83 ms
64 bytes from 8.8.8.8: icmp_seq=2 ttl=115 time=10.2 ms

--- 8.8.8.8 ping statistics ---
2 packets transmitted, 2 received, 0% packet loss, time 1001ms
rtt min/avg/max/mdev = 9.830/10.015/10.200/0.185 ms`

	result := &PingResult{}
	parseHostPingOutput(output, result)

	if result.Sent != 2 || result.Received != 2 || result.Lost != 0 {
		t.Fatalf("unexpected counters: %+v", result)
	}
	if len(result.Replies) != 2 || result.Replies[0].From != "8.8.8.8" || result.Replies[0].TTL != 115 {
		t.Fatalf("unexpected replies: %+v", result.Replies)
	}
	if result.AvgMs != 10.015 {
		t.Fatalf("unexpected avg: %v", result.AvgMs)
	}
}

func TestParseHostPingOutputMacOS(t *testing.T) {
	output := `PING 8.8.8.8 (8.8.8.8): 56 data bytes
64 bytes from 8.8.8.8: icmp_seq=0 ttl=115 time=12.345 ms

--- 8.8.8.8 ping statistics ---
1 packets transmitted, 1 packets received, 0.0% packet loss
round-trip min/avg/max/stddev = 12.345/12.345/12.345/0.000 ms`

	result := &PingResult{}
	parseHostPingOutput(output, result)

	if result.Sent != 1 || result.Received != 1 || result.AvgMs != 12.345 {
		t.Fatalf("unexpected result: %+v", result)
	}
}

func TestParseHostPingOutputWindows(t *testing.T) {
	output := `Pinging 8.8.8.8 with 32 bytes of data:
Reply from 8.8.8.8: bytes=32 time=9ms TTL=115
Reply from 8.8.8.8: bytes=32 time=10ms TTL=115

Ping statistics for 8.8.8.8:
    Packets: Sent = 2, Received = 2, Lost = 0 (0% loss),
Approximate round trip times in milli-seconds:
    Minimum = 9ms, Maximum = 10ms, Average = 9ms`

	result := &PingResult{}
	parseHostPingOutput(output, result)

	if result.Sent != 2 || result.Received != 2 {
		t.Fatalf("unexpected counters: %+v", result)
	}
	if result.MinMs != 9 || result.MaxMs != 10 || result.AvgMs != 9 {
		t.Fatalf("unexpected rtt stats: %+v", result)
	}
}

func TestParseHostPingOutputLoss(t *testing.T) {
	output := `PING 10.255.255.1 (10.255.255.1) 56(84) bytes of data.

--- 10.255.255.1 ping statistics ---
2 packets transmitted, 0 received, 100% packet loss, time 1015ms`

	result := &PingResult{}
	parseHostPingOutput(output, result)

	if result.Sent != 2 || result.Received != 0 || result.Lost != 2 {
		t.Fatalf("unexpected counters: %+v", result)
	}
	if len(result.Replies) != 0 {
		t.Fatalf("expected no replies, got %+v", result.Replies)
	}
}

func TestOrderPingVariants(t *testing.T) {
	ordered := orderPingVariants("QUECTEL EC200A")
	if ordered[0].key != "quectel" {
		t.Errorf("quectel should come first, got %s", ordered[0].key)
	}

	ordered = orderPingVariants("CHINA MOBILE CM")
	if ordered[0].key != "cmcc" {
		t.Errorf("cmcc should come first, got %s", ordered[0].key)
	}

	// 未知厂商保持原始顺序且不丢失变体
	ordered = orderPingVariants("")
	if len(ordered) != len(pingVariants) {
		t.Errorf("expected %d variants, got %d", len(pingVariants), len(ordered))
	}
}
