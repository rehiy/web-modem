package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestSetNoCache 验证动态接口设置了禁止缓存的响应头，
// 防止浏览器/反向代理缓存设备列表等实时数据。
func TestSetNoCache(t *testing.T) {
	rec := httptest.NewRecorder()
	setNoCache(rec)

	if got := rec.Header().Get("Cache-Control"); got != "no-store, no-cache, must-revalidate" {
		t.Fatalf("unexpected Cache-Control: %q", got)
	}
	if got := rec.Header().Get("Pragma"); got != "no-cache" {
		t.Fatalf("unexpected Pragma: %q", got)
	}
	if got := rec.Header().Get("Expires"); got != "0" {
		t.Fatalf("unexpected Expires: %q", got)
	}
}

func TestCheckUpdateSetsNoCache(t *testing.T) {
	handler := NewUpdateHandler()
	request := httptest.NewRequest(http.MethodGet, "/api/update/check", nil)
	request = request.WithContext(context.Background())
	recorder := httptest.NewRecorder()

	// 使用已取消的请求上下文，避免测试访问外部更新服务；这里只验证响应头。
	ctx, cancel := context.WithCancel(request.Context())
	cancel()
	handler.CheckUpdate(recorder, request.WithContext(ctx))

	if got := recorder.Header().Get("Cache-Control"); got != "no-store, no-cache, must-revalidate" {
		t.Fatalf("unexpected Cache-Control: %q", got)
	}
	if got := recorder.Header().Get("Pragma"); got != "no-cache" {
		t.Fatalf("unexpected Pragma: %q", got)
	}
}
