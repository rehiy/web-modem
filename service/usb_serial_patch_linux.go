//go:build linux

package service

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const (
	defaultModemUSBIDs          = "2ecc:3012"
	defaultDisabledIfaceMACs    = "ac:0c:29:a3:9b:6d"
	optionDriverNewIDPath       = "/sys/bus/usb-serial/drivers/option1/new_id"
	usbSerialPatchRetryInterval = 15 * time.Second
)

var (
	usbSerialPatchMu      sync.Mutex
	lastUSBSerialPatchRun time.Time
)

// EnsureUSBSerialDrivers binds known USB modem IDs to the Linux option driver.
// This mirrors patch.sh so hot-plugged devices can appear during normal scans.
func EnsureUSBSerialDrivers() {
	usbSerialPatchMu.Lock()
	defer usbSerialPatchMu.Unlock()

	if time.Since(lastUSBSerialPatchRun) < usbSerialPatchRetryInterval {
		return
	}
	lastUSBSerialPatchRun = time.Now()

	if err := runModprobeOption(); err != nil {
		log.Printf("[USB] failed to load option driver: %v", err)
	}

	usbIDsRaw, ok := os.LookupEnv("MODEM_USB_IDS")
	if !ok {
		usbIDsRaw = defaultModemUSBIDs
	}
	for _, id := range parseUSBSerialIDs(usbIDsRaw) {
		if err := bindOptionDriverID(id); err != nil {
			log.Printf("[USB] failed to bind option driver id %s: %v", id, err)
		}
	}

	macsRaw, ok := os.LookupEnv("MODEM_DISABLE_IFACE_MACS")
	if !ok {
		macsRaw = defaultDisabledIfaceMACs
	}
	for _, mac := range parseMACList(macsRaw) {
		if err := disableInterfacesByMAC(mac); err != nil {
			log.Printf("[USB] failed to disable interface with MAC %s: %v", mac, err)
		}
	}
}

func runModprobeOption() error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	output, err := exec.CommandContext(ctx, "modprobe", "option").CombinedOutput()
	if err != nil {
		return fmt.Errorf("%w: %s", err, strings.TrimSpace(string(output)))
	}
	return nil
}

func bindOptionDriverID(id string) error {
	if _, err := os.Stat(optionDriverNewIDPath); err != nil {
		return err
	}

	err := os.WriteFile(optionDriverNewIDPath, []byte(strings.ReplaceAll(id, ":", " ")+"\n"), 0200)
	if err == nil || strings.Contains(strings.ToLower(err.Error()), "file exists") {
		return nil
	}
	return err
}

func parseUSBSerialIDs(raw string) []string {
	fields := strings.FieldsFunc(raw, func(r rune) bool {
		return r == ',' || r == ';' || r == '\n' || r == '\t' || r == ' '
	})

	ids := make([]string, 0, len(fields))
	for i := 0; i < len(fields); i++ {
		field := strings.TrimSpace(fields[i])
		if field == "" {
			continue
		}

		if vendor, product, ok := splitUSBID(field); ok {
			ids = append(ids, vendor+":"+product)
			continue
		}

		if i+1 < len(fields) {
			vendor := normalizeHexID(field)
			product := normalizeHexID(fields[i+1])
			if vendor != "" && product != "" {
				ids = append(ids, vendor+":"+product)
				i++
			}
		}
	}
	return dedupeStrings(ids)
}

func splitUSBID(value string) (string, string, bool) {
	for _, sep := range []string{":", "/"} {
		if vendor, product, ok := strings.Cut(value, sep); ok {
			vendor = normalizeHexID(vendor)
			product = normalizeHexID(product)
			return vendor, product, vendor != "" && product != ""
		}
	}
	return "", "", false
}

func normalizeHexID(value string) string {
	value = strings.ToLower(strings.TrimSpace(strings.TrimPrefix(value, "0x")))
	if len(value) == 0 || len(value) > 4 {
		return ""
	}
	for _, r := range value {
		if (r < '0' || r > '9') && (r < 'a' || r > 'f') {
			return ""
		}
	}
	return value
}

func parseMACList(raw string) []string {
	fields := strings.FieldsFunc(raw, func(r rune) bool {
		return r == ',' || r == ';' || r == '\n' || r == '\t' || r == ' '
	})

	macs := make([]string, 0, len(fields))
	for _, field := range fields {
		if mac := strings.ToLower(strings.TrimSpace(field)); mac != "" {
			macs = append(macs, mac)
		}
	}
	return dedupeStrings(macs)
}

func disableInterfacesByMAC(mac string) error {
	matches, err := filepath.Glob("/sys/class/net/*/address")
	if err != nil {
		return err
	}

	var errs []error
	for _, addressPath := range matches {
		data, err := os.ReadFile(addressPath)
		if err != nil {
			continue
		}
		if strings.ToLower(strings.TrimSpace(string(data))) != mac {
			continue
		}

		iface := filepath.Base(filepath.Dir(addressPath))
		if iface == "" || iface == "lo" {
			continue
		}
		if err := setInterfaceDown(iface); err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", iface, err))
			continue
		}
		log.Printf("[USB] disabled network interface %s with MAC %s", iface, mac)
	}
	return errors.Join(errs...)
}

func setInterfaceDown(iface string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	output, err := exec.CommandContext(ctx, "ip", "link", "set", iface, "down").CombinedOutput()
	if err != nil {
		return fmt.Errorf("%w: %s", err, strings.TrimSpace(string(output)))
	}
	return nil
}

func dedupeStrings(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}
