package handler

import (
	"os"
	"runtime"
	"testing"
)

// TestRestartTargetPathPrefersExplicitPath 验证显式传入的路径优先于
// os.Executable，这是更新后正确重启的关键：更新流程会让 os.Executable
// 漂移到旧版本路径，必须用更新前捕获的路径才能加载到新二进制。
func TestRestartTargetPathPrefersExplicitPath(t *testing.T) {
	if got := restartTargetPath("/usr/local/bin/web-modem"); got != "/usr/local/bin/web-modem" {
		t.Fatalf("expected explicit path to be used, got %q", got)
	}
}

// TestRestartTargetPathFallsBackToExecutable 验证未传入路径时回退到
// os.Executable，供普通重启（未发生重命名）场景使用。
func TestRestartTargetPathFallsBackToExecutable(t *testing.T) {
	exe, err := os.Executable()
	if err != nil {
		t.Skipf("os.Executable unavailable: %v", err)
	}
	if got := restartTargetPath(""); got != exe {
		t.Fatalf("expected fallback to os.Executable, got %q want %q", got, exe)
	}
}

// TestOsExecutableDriftsAfterRename 复现并固化更新流程的根因：
// 把运行中的二进制重命名为 backup 后，/proc/self/exe（os.Executable）
// 会跟随重命名指向旧版本路径。
//
// 因此 Apply 之后若再调用 os.Executable 将得到旧版本路径，重启会再次
// 加载旧二进制——这也是「应用后服务重启但版本未变，需要二次应用」的
// 直接原因。handler 必须在 Apply 之前捕获路径，重启时显式传入。
func TestOsExecutableDriftsAfterRename(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("可执行路径随重命名漂移是 Linux /proc/self/exe 行为")
	}
	original, err := os.Executable()
	if err != nil {
		t.Skipf("os.Executable unavailable: %v", err)
	}

	backup := original + ".bak"
	_ = os.Remove(backup)
	if err := os.Rename(original, backup); err != nil {
		t.Skipf("cannot rename test binary: %v", err)
	}
	t.Cleanup(func() { _ = os.Rename(backup, original) })

	// 模拟更新后场景：旧二进制已被改名为 backup。
	drifted, err := os.Executable()
	if err != nil {
		t.Fatalf("os.Executable after rename: %v", err)
	}
	if drifted == original {
		t.Fatalf("expected os.Executable() to drift to backup path, still %q", original)
	}

	// 回退逻辑（空路径）拿到的是漂移后的旧版本路径——这正是更新后
	// 错误重启的来源。
	if got := restartTargetPath(""); got != drifted {
		t.Fatalf("expected fallback to drift to backup, got %q want %q", got, drifted)
	}

	// 显式传入更新前捕获的路径，才能定位到 original 处的新二进制。
	if got := restartTargetPath(original); got != original {
		t.Fatalf("expected explicit original path, got %q", got)
	}
}
