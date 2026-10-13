package handler

import "testing"

func TestNormalizeICCID(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  string
	}{
		{name: "标准响应", value: "+ICCID: 89860012345678901234", want: "89860012345678901234"},
		{name: "错误响应", value: "ERROR", want: ""},
		{name: "位数不足", value: "123456", want: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := normalizeICCID(tt.value); got != tt.want {
				t.Fatalf("normalizeICCID(%q) = %q, want %q", tt.value, got, tt.want)
			}
		})
	}
}

func TestSimStatusLabelError(t *testing.T) {
	if got := simStatusLabel("ERROR"); got != "读取失败" {
		t.Fatalf("simStatusLabel(ERROR) = %q", got)
	}
}
