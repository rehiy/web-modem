package service

import "testing"

func TestNormalizeAlertDigits(t *testing.T) {
	if got := normalizeAlertDigits(" +8613800-abc "); got != "8613800" {
		t.Fatalf("unexpected normalized digits: %s", got)
	}
}
