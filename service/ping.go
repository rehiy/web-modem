package service

import (
	"context"
	"fmt"
	"net"
	"os/exec"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"
)

// pingURCPrefixes 需要注册到 AT 库 CustomNotify 的厂商 ping URC 前缀。
// 这些上报在最终 OK 之后异步到达，不注册会被当作残留响应丢弃。
var pingURCPrefixes = []string{"+QPING", "+MPING", "+CPING"}

// defaultPingHost 未指定目标时的默认地址
const defaultPingHost = "8.8.8.8"

// maxPingCount 单次测试的最大包数，避免长时间占用 AT 通道
const maxPingCount = 10

// PingReply 单个 ping 应答
type PingReply struct {
	From    string  `json:"from,omitempty"`    // 应答来源 IP
	TimeMs  float64 `json:"time_ms"`           // 往返时延（毫秒）
	TTL     int     `json:"ttl,omitempty"`     // 应答 TTL
	Timeout bool    `json:"timeout,omitempty"` // 该包是否超时
}

// PingResult 一次连通性测试的结果
type PingResult struct {
	Source   string      `json:"source"`              // modem=模组蜂窝链路 host=本机网络
	Command  string      `json:"command"`             // 实际执行的命令
	Host     string      `json:"host"`                // 测试目标
	Sent     int         `json:"sent"`                // 发送包数
	Received int         `json:"received"`            // 接收包数
	Lost     int         `json:"lost"`                // 丢失包数
	MinMs    float64     `json:"min_ms,omitempty"`    // 最小时延
	AvgMs    float64     `json:"avg_ms,omitempty"`    // 平均时延
	MaxMs    float64     `json:"max_ms,omitempty"`    // 最大时延
	Replies  []PingReply `json:"replies"`             // 逐包结果
	Raw      []string    `json:"raw"`                 // 原始输出
	Finished bool        `json:"finished"`            // 是否收到完整统计（false 表示等待超时截断）
	ErrorMsg string      `json:"error_msg,omitempty"` // 模组上报的错误说明
}

// pingVariant 厂商私有 ping 指令变体
type pingVariant struct {
	key     string                              // 解析分支标识
	urc     string                              // 结果 URC 前缀
	vendors []string                            // 制造商关键字（AT+CGMI 响应，大写匹配）
	build   func(host string, count int) string // 构造 AT 指令
}

// pingVariants 按厂商定义的 ping 指令表。制造商匹配的变体优先，
// 未匹配时依次尝试，指令不被支持（ERROR）则降级到下一个。
var pingVariants = []pingVariant{
	{
		key:     "quectel",
		urc:     "+QPING",
		vendors: []string{"QUECTEL"},
		build: func(host string, count int) string {
			// contextID=1，单包超时 4 秒
			return fmt.Sprintf("AT+QPING=1,%q,4,%d", host, count)
		},
	},
	{
		key:     "cmcc",
		urc:     "+MPING",
		vendors: []string{"CMCC", "CHINA MOBILE", "CMIT", "CMDZ", "ONEMO", "ML307"},
		build: func(host string, count int) string {
			// AT+MPING=<host>[,<timeout>[,<ping_num>[,<packet_len>[,<cid>]]]]，单包超时 4 秒
			return fmt.Sprintf("AT+MPING=%q,4,%d", host, count)
		},
	},
	{
		key:     "simcom",
		urc:     "+CPING",
		vendors: []string{"SIMCOM"},
		build: func(host string, count int) string {
			// dest_type=1 表示 IPv4
			return fmt.Sprintf("AT+CPING=%q,1,%d", host, count)
		},
	},
}

var pingHostRegex = regexp.MustCompile(`^[A-Za-z0-9]([A-Za-z0-9.-]{0,252}[A-Za-z0-9])?$`)

// IsValidPingHost 校验 ping 目标是否为合法 IP 或主机名，
// 防止把特殊字符注入 AT 指令或命令行参数。
func IsValidPingHost(host string) bool {
	if net.ParseIP(host) != nil {
		return true
	}
	return pingHostRegex.MatchString(host)
}

// normalizePingTarget 规范化目标地址与包数
func normalizePingTarget(host string, count int) (string, int, error) {
	host = strings.TrimSpace(host)
	if host == "" {
		host = defaultPingHost
	}
	if !IsValidPingHost(host) {
		return "", 0, fmt.Errorf("无效的目标地址: %s", host)
	}
	if count <= 0 {
		count = 4
	}
	if count > maxPingCount {
		count = maxPingCount
	}
	return host, count, nil
}

// PingFromModem 通过模组厂商 AT 指令执行 ping，检测蜂窝数据链路连通性。
// 按制造商选择指令变体，不支持时依次降级尝试。
func (m *ModemService) PingFromModem(name, host string, count int) (*PingResult, error) {
	host, count, err := normalizePingTarget(host, count)
	if err != nil {
		return nil, err
	}

	conn, err := m.GetConn(name)
	if err != nil {
		return nil, err
	}

	manufacturer := ""
	if v, err := conn.GetManufacturer(); err == nil {
		manufacturer = strings.ToUpper(v)
	}

	tried := []string{}
	for _, variant := range orderPingVariants(manufacturer) {
		cmd := variant.build(host, count)
		result, supported := m.tryPingVariant(conn, variant, cmd, host, count)
		if supported {
			return result, nil
		}
		tried = append(tried, cmd)
	}
	return nil, fmt.Errorf("模组不支持已知的 ping 指令，已尝试: %s", strings.Join(tried, " / "))
}

// orderPingVariants 返回按制造商优先排序的指令变体列表
func orderPingVariants(manufacturer string) []pingVariant {
	ordered := make([]pingVariant, 0, len(pingVariants))
	rest := make([]pingVariant, 0, len(pingVariants))
	for _, variant := range pingVariants {
		matched := false
		for _, vendor := range variant.vendors {
			if manufacturer != "" && strings.Contains(manufacturer, vendor) {
				matched = true
				break
			}
		}
		if matched {
			ordered = append(ordered, variant)
		} else {
			rest = append(rest, variant)
		}
	}
	return append(ordered, rest...)
}

// tryPingVariant 发送指令并收集 URC 结果。
// 第二个返回值表示模组是否接受了该指令（false 时调用方降级尝试下一变体）。
func (m *ModemService) tryPingVariant(conn *ModemConn, variant pingVariant, cmd, host string, count int) (*PingResult, bool) {
	// 先注册 URC 监听再发命令，避免早到的上报丢失
	ch, cancel := m.WatchURC(conn.Name)
	defer cancel()

	responses, err := conn.SendCommandWithTimeout(cmd, 8*time.Second)
	if err != nil || atLinesHaveError(responses) {
		return nil, false
	}

	result := &PingResult{
		Source:  "modem",
		Command: cmd,
		Host:    host,
		Replies: []PingReply{},
		Raw:     append([]string{}, responses...),
	}

	// 单包超时 4 秒 + 收尾余量
	deadline := time.After(time.Duration(count)*4*time.Second + 6*time.Second)
collect:
	for !result.Finished {
		select {
		case urc := <-ch:
			if !URCMatches(urc.Event, variant.urc) {
				continue
			}
			params := urcParamsToSlice(urc.Params)
			result.Raw = append(result.Raw, formatURCLine(urc.Event, params))
			applyPingURC(variant.key, params, result)
			// ML307 在 ping_num=1 时不上报统计行，收到单包结果即完成
			if variant.key == "cmcc" && count == 1 && (len(result.Replies) > 0 || result.ErrorMsg != "") {
				result.Finished = true
			}
		case <-deadline:
			// 等待超时，截断返回已收集的结果
			break collect
		}
	}

	// AT 库对每条 URC 用独立 goroutine 分发，最后一包与统计行可能乱序到达，
	// 统计行之后再短暂收尾，避免漏记压线的逐包结果
	if result.Finished {
		grace := time.After(500 * time.Millisecond)
	drain:
		for {
			select {
			case urc := <-ch:
				if !URCMatches(urc.Event, variant.urc) {
					continue
				}
				params := urcParamsToSlice(urc.Params)
				result.Raw = append(result.Raw, formatURCLine(urc.Event, params))
				applyPingURC(variant.key, params, result)
			case <-grace:
				break drain
			}
		}
	}

	finalizePingResult(result)
	return result, true
}

// applyPingURC 按厂商变体解析单条 ping URC，写入结果
func applyPingURC(variantKey string, params []string, result *PingResult) {
	switch variantKey {
	case "quectel":
		applyQuectelPingURC(params, result)
	case "cmcc":
		applyCmccPingURC(params, result)
	case "simcom":
		applySimcomPingURC(params, result)
	}
}

// applyQuectelPingURC 解析移远 +QPING 上报。
// 逐包: +QPING: 0,"8.8.8.8",32,161,255（result,ip,bytes,time,ttl）
// 统计: +QPING: 0,4,4,0,161,170,165（finresult,sent,rcvd,lost,min,max,avg）
// 错误: +QPING: <err>（单参数非 0，如 565 域名解析失败）
func applyQuectelPingURC(params []string, result *PingResult) {
	switch {
	case len(params) >= 7:
		result.Sent = parsePingInt(params[1])
		result.Received = parsePingInt(params[2])
		result.Lost = parsePingInt(params[3])
		result.MinMs = parsePingFloat(params[4])
		result.MaxMs = parsePingFloat(params[5])
		result.AvgMs = parsePingFloat(params[6])
		result.Finished = true
	case len(params) >= 5 && params[0] == "0":
		result.Replies = append(result.Replies, PingReply{
			From:   params[1],
			TimeMs: parsePingFloat(params[3]),
			TTL:    parsePingInt(params[4]),
		})
	case len(params) == 1 && params[0] != "0":
		code := parsePingInt(params[0])
		if code <= 2 {
			// 1/2 为单包超时类结果
			result.Replies = append(result.Replies, PingReply{Timeout: true})
		} else {
			result.ErrorMsg = fmt.Sprintf("ping 失败，错误码 %s", params[0])
			result.Finished = true
		}
	}
}

// applyCmccPingURC 解析中移 ML307 +MPING 上报。
// 逐包: +MPING: <result>[,<ip>,<packet_len>,<time>,<ttl>]，result=0 成功，
// 失败时仅有单参数错误码（1=DNS 解析失败 2=DNS 解析超时 3=响应错误 4=响应超时 5=其他）
// 统计: +MPING: "statistics",<sent>,<lost>,<rtt_min>,<rtt_max>,<rtt_avg>
func applyCmccPingURC(params []string, result *PingResult) {
	switch {
	case len(params) >= 6 && strings.EqualFold(params[0], "statistics"):
		result.Sent = parsePingInt(params[1])
		result.Lost = parsePingInt(params[2])
		result.Received = result.Sent - result.Lost
		result.MinMs = parsePingFloat(params[3])
		result.MaxMs = parsePingFloat(params[4])
		result.AvgMs = parsePingFloat(params[5])
		result.Finished = true
	case len(params) >= 5 && params[0] == "0" && looksLikeIP(params[1]):
		result.Replies = append(result.Replies, PingReply{
			From:   params[1],
			TimeMs: parsePingFloat(params[3]),
			TTL:    parsePingInt(params[4]),
		})
	case len(params) == 1 && params[0] != "0":
		switch parsePingInt(params[0]) {
		case 3, 4:
			// 响应错误/响应超时，按单包丢失计
			result.Replies = append(result.Replies, PingReply{Timeout: true})
		case 1:
			result.ErrorMsg = "ping 失败：DNS 解析失败"
			result.Finished = true
		case 2:
			result.ErrorMsg = "ping 失败：DNS 解析超时"
			result.Finished = true
		default:
			result.ErrorMsg = fmt.Sprintf("ping 失败，错误码 %s", params[0])
			result.Finished = true
		}
	}
}

// applySimcomPingURC 解析 SIMCom +CPING 上报。
// 逐包: +CPING: 1,<ip>,<bytes>,<rtt>,<ttl>；超时: +CPING: 2
// 统计: +CPING: 3,<sent>,<rcvd>,<lost>,<min>,<max>,<avg>
func applySimcomPingURC(params []string, result *PingResult) {
	if len(params) == 0 {
		return
	}
	switch params[0] {
	case "1":
		if len(params) >= 5 {
			result.Replies = append(result.Replies, PingReply{
				From:   params[1],
				TimeMs: parsePingFloat(params[3]),
				TTL:    parsePingInt(params[4]),
			})
		}
	case "2":
		result.Replies = append(result.Replies, PingReply{Timeout: true})
	case "3":
		if len(params) >= 7 {
			result.Sent = parsePingInt(params[1])
			result.Received = parsePingInt(params[2])
			result.Lost = parsePingInt(params[3])
			result.MinMs = parsePingFloat(params[4])
			result.MaxMs = parsePingFloat(params[5])
			result.AvgMs = parsePingFloat(params[6])
		}
		result.Finished = true
	}
}

// finalizePingResult 补全缺失的统计字段（未收到统计 URC 时按逐包结果推算）
func finalizePingResult(result *PingResult) {
	if result.Sent == 0 {
		result.Sent = len(result.Replies)
	}
	received := 0
	for _, reply := range result.Replies {
		if !reply.Timeout {
			received++
		}
	}
	if result.Received == 0 {
		result.Received = received
	}
	if result.Lost == 0 && result.Sent >= result.Received {
		result.Lost = result.Sent - result.Received
	}
	if result.AvgMs == 0 && received > 0 {
		min, max, sum := 0.0, 0.0, 0.0
		first := true
		for _, reply := range result.Replies {
			if reply.Timeout {
				continue
			}
			if first || reply.TimeMs < min {
				min = reply.TimeMs
			}
			if first || reply.TimeMs > max {
				max = reply.TimeMs
			}
			sum += reply.TimeMs
			first = false
		}
		result.MinMs = min
		result.MaxMs = max
		result.AvgMs = sum / float64(received)
	}
}

// PingFromHost 在运行本服务的主机上执行系统 ping，检测主机网络连通性
func PingFromHost(host string, count int) (*PingResult, error) {
	host, count, err := normalizePingTarget(host, count)
	if err != nil {
		return nil, err
	}

	var args []string
	switch runtime.GOOS {
	case "windows":
		args = []string{"-n", strconv.Itoa(count), "-w", "3000", host}
	case "darwin":
		args = []string{"-c", strconv.Itoa(count), host}
	default:
		args = []string{"-c", strconv.Itoa(count), "-W", "3", host}
	}

	ctx, cancelCtx := context.WithTimeout(context.Background(), time.Duration(count)*4*time.Second+5*time.Second)
	defer cancelCtx()

	// 参数独立传递，不经过 shell，目标地址已通过白名单校验
	out, err := exec.CommandContext(ctx, "ping", args...).CombinedOutput()
	output := strings.TrimSpace(string(out))
	if output == "" && err != nil {
		return nil, fmt.Errorf("执行 ping 失败: %w", err)
	}

	result := &PingResult{
		Source:   "host",
		Command:  "ping " + strings.Join(args, " "),
		Host:     host,
		Replies:  []PingReply{},
		Raw:      strings.Split(output, "\n"),
		Finished: true,
	}
	parseHostPingOutput(output, result)
	return result, nil
}

var (
	hostPingReplyRegex   = regexp.MustCompile(`(?i)(?:time|时间)[=<]\s*([0-9.]+)\s*ms`)
	hostPingFromRegex    = regexp.MustCompile(`(?i)(?:from|来自) ([0-9a-fA-F.:]+)`)
	hostPingTTLRegex     = regexp.MustCompile(`(?i)ttl[=:]([0-9]+)`)
	hostPingUnixStat     = regexp.MustCompile(`(\d+) packets transmitted, (\d+)(?: packets)? received`)
	hostPingWinStat      = regexp.MustCompile(`(?:Sent|已发送) = (\d+).*(?:Received|已接收) = (\d+)`)
	hostPingUnixRttRegex = regexp.MustCompile(`min/avg/max[^=]*= ([0-9.]+)/([0-9.]+)/([0-9.]+)`)
	hostPingWinRttRegex  = regexp.MustCompile(`(?:Minimum|最短) = ([0-9]+)ms.*(?:Maximum|最长) = ([0-9]+)ms.*(?:Average|平均) = ([0-9]+)ms`)
)

// parseHostPingOutput 尽力解析系统 ping 输出（兼容 Linux/macOS/Windows，含中文本地化），
// 解析失败时原始输出仍完整保留在 Raw 中
func parseHostPingOutput(output string, result *PingResult) {
	for _, line := range strings.Split(output, "\n") {
		match := hostPingReplyRegex.FindStringSubmatch(line)
		if match == nil {
			continue
		}
		reply := PingReply{TimeMs: parsePingFloat(match[1])}
		if from := hostPingFromRegex.FindStringSubmatch(line); from != nil {
			reply.From = strings.TrimSuffix(from[1], ":")
		}
		if ttl := hostPingTTLRegex.FindStringSubmatch(line); ttl != nil {
			reply.TTL = parsePingInt(ttl[1])
		}
		result.Replies = append(result.Replies, reply)
	}

	if match := hostPingUnixStat.FindStringSubmatch(output); match != nil {
		result.Sent = parsePingInt(match[1])
		result.Received = parsePingInt(match[2])
	} else if match := hostPingWinStat.FindStringSubmatch(output); match != nil {
		result.Sent = parsePingInt(match[1])
		result.Received = parsePingInt(match[2])
	}

	if match := hostPingUnixRttRegex.FindStringSubmatch(output); match != nil {
		result.MinMs = parsePingFloat(match[1])
		result.AvgMs = parsePingFloat(match[2])
		result.MaxMs = parsePingFloat(match[3])
	} else if match := hostPingWinRttRegex.FindStringSubmatch(output); match != nil {
		result.MinMs = parsePingFloat(match[1])
		result.MaxMs = parsePingFloat(match[2])
		result.AvgMs = parsePingFloat(match[3])
	}

	finalizePingResult(result)
}

// ===== 工具函数 =====

// atLinesHaveError 判断 AT 响应是否以错误结束
func atLinesHaveError(lines []string) bool {
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "ERROR") || strings.HasPrefix(line, "+CME ERROR") || strings.HasPrefix(line, "+CMS ERROR") {
			return true
		}
	}
	return false
}

// urcParamsToSlice 把 URC 参数 map 按序号还原为有序切片
func urcParamsToSlice(params map[int]string) []string {
	keys := make([]int, 0, len(params))
	for key := range params {
		keys = append(keys, key)
	}
	sort.Ints(keys)
	values := make([]string, 0, len(keys))
	for _, key := range keys {
		values = append(values, params[key])
	}
	return values
}

// formatURCLine 还原 URC 行用于原始输出展示
func formatURCLine(event string, params []string) string {
	if len(params) == 0 {
		return event
	}
	return event + ": " + strings.Join(params, ",")
}

func parsePingInt(value string) int {
	n, _ := strconv.Atoi(strings.TrimSpace(value))
	return n
}

func parsePingFloat(value string) float64 {
	f, _ := strconv.ParseFloat(strings.TrimSpace(value), 64)
	return f
}

func looksLikeIP(value string) bool {
	return strings.Contains(value, ".") || strings.Contains(value, ":")
}
