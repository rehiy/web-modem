package service

import (
	"errors"
	"testing"
)

func TestIsAcceptedProfileSwitchStatus(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{
			name: "91xx 表示已接受的 Proactive SIM 响应",
			err:  errors.New("启用 Profile: returned an unexpected response with status 910B"),
			want: true,
		},
		{
			name: "其他 APDU 状态不视为成功",
			err:  errors.New("启用 Profile: returned an unexpected response with status 6A80"),
			want: false,
		},
		{
			name: "非状态字错误不视为成功",
			err:  errors.New("启用 Profile: CSIM response too short"),
			want: false,
		},
		{
			name: "空错误不视为状态响应",
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isAcceptedProfileSwitchStatus(tt.err); got != tt.want {
				t.Fatalf("isAcceptedProfileSwitchStatus() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestIsAcceptedProfileSwitchInterruption(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{
			name: "CSIM 无响应视为切卡复位中断",
			err:  errors.New("启用 Profile: CSIM: no response"),
			want: true,
		},
		{
			name: "模块 CME 错误视为切卡复位中断",
			err:  errors.New("启用 Profile: CME ERROR: 3"),
			want: true,
		},
		{
			name: "业务状态错误不能视为成功",
			err:  errors.New("启用 Profile: returned an unexpected response with status 6A80"),
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isAcceptedProfileSwitchInterruption(tt.err); got != tt.want {
				t.Fatalf("isAcceptedProfileSwitchInterruption() = %v, want %v", got, tt.want)
			}
		})
	}
}
