package service

import (
	"log"
	"net"
	"os"
	"sort"
	"strings"
	"sync"
	"time"
)

var (
	lastKnownIPs  []string
	lastIPsMu     sync.Mutex
	ipMonitorOnce sync.Once
)

const defaultIPMonitorIgnoreMACs = "ac:0c:29:a3:9b:6d"

func getLocalIPs() []string {
	ifaces, err := net.Interfaces()
	if err != nil {
		log.Printf("[Network] Failed to get interfaces: %v", err)
		return nil
	}

	ignoredMACs := ipMonitorIgnoredMACs()
	var ips []string
	for _, iface := range ifaces {
		if !shouldMonitorInterface(iface, ignoredMACs) {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			log.Printf("[Network] Failed to get addrs for %s: %v", iface.Name, err)
			continue
		}
		for _, addr := range addrs {
			ipNet, ok := addr.(*net.IPNet)
			if !ok || ipNet.IP.IsLoopback() || !ipNet.IP.IsGlobalUnicast() {
				continue
			}
			if ipNet.IP.To4() == nil {
				continue
			}
			ips = append(ips, ipNet.IP.String())
		}
	}
	sort.Strings(ips)
	return ips
}

func shouldMonitorInterface(iface net.Interface, ignoredMACs map[string]struct{}) bool {
	if iface.Flags&net.FlagLoopback != 0 || iface.Flags&net.FlagUp == 0 {
		return false
	}

	mac := strings.ToLower(iface.HardwareAddr.String())
	if mac == "" {
		return true
	}
	_, ignored := ignoredMACs[mac]
	return !ignored
}

func ipMonitorIgnoredMACs() map[string]struct{} {
	raw, ok := os.LookupEnv("IP_MONITOR_IGNORE_MACS")
	if !ok {
		raw, ok = os.LookupEnv("MODEM_DISABLE_IFACE_MACS")
	}
	if !ok {
		raw = defaultIPMonitorIgnoreMACs
	}

	macs := make(map[string]struct{})
	for _, field := range strings.FieldsFunc(raw, func(r rune) bool {
		return r == ',' || r == ';' || r == '\n' || r == '\t' || r == ' '
	}) {
		mac := strings.ToLower(strings.TrimSpace(field))
		if mac == "" {
			continue
		}
		macs[mac] = struct{}{}
	}
	return macs
}

func ipSliceDiff(a, b []string) []string {
	bSet := make(map[string]struct{}, len(b))
	for _, s := range b {
		bSet[s] = struct{}{}
	}
	var result []string
	for _, s := range a {
		if _, ok := bSet[s]; !ok {
			result = append(result, s)
		}
	}
	return result
}

func checkIPChanges() {
	current := getLocalIPs()

	lastIPsMu.Lock()
	last := lastKnownIPs
	if last == nil {
		lastKnownIPs = current
		lastIPsMu.Unlock()
		return
	}
	if strings.Join(current, ",") == strings.Join(last, ",") {
		lastIPsMu.Unlock()
		return
	}
	lastKnownIPs = current
	lastIPsMu.Unlock()

	added := ipSliceDiff(current, last)
	removed := ipSliceDiff(last, current)

	log.Printf("[Network] IP changed: old=%v new=%v added=%v removed=%v", last, current, added, removed)
	NewWebhookService().HandleDataEvent(WebhookEventIPChanged, map[string]any{
		"old_ips":     strings.Join(last, ", "),
		"new_ips":     strings.Join(current, ", "),
		"added_ips":   strings.Join(added, ", "),
		"removed_ips": strings.Join(removed, ", "),
	})
}

// StartIPMonitor 启动后台 IP 变化监控。
func StartIPMonitor(interval time.Duration) {
	ipMonitorOnce.Do(func() {
		go func() {
			checkIPChanges()
			for {
				time.Sleep(interval)
				checkIPChanges()
			}
		}()
	})
}
