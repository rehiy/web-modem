package handler

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"syscall"
	"time"

	"github.com/rehiy/libgo/upgrade"
	"github.com/rehiy/web-modem/service"
)

// UpdateHandler 自更新处理器
type UpdateHandler struct {
	updateService *service.UpdateService
}

// NewUpdateHandler 创建新的自更新处理器
func NewUpdateHandler() *UpdateHandler {
	return &UpdateHandler{updateService: service.NewUpdateService()}
}

// CheckUpdate 检查是否存在新版本
func (h *UpdateHandler) CheckUpdate(w http.ResponseWriter, r *http.Request) {
	setNoCache(w)

	result, err := h.updateService.Check(r.Context())
	if err != nil {
		respondJSON(w, http.StatusInternalServerError, H{"error": err.Error()})
		return
	}

	respondJSON(w, http.StatusOK, result)
}

// ApplyUpdate 下载并应用新版本
func (h *UpdateHandler) ApplyUpdate(w http.ResponseWriter, r *http.Request) {
	setNoCache(w)

	var req struct {
		Restart bool `json:"restart"`
	}

	if r.Body != nil {
		_ = json.NewDecoder(r.Body).Decode(&req)
	}

	// 必须在 Apply 之前捕获当前可执行路径：Apply 内部会把运行中的二进制
	// 重命名为 backup，随后 /proc/self/exe（os.Executable）会跟随重命名
	// 指向旧版本路径，此时重启将再次加载旧二进制，表现为「应用后服务
	// 重启但版本未变」。新二进制最终写入的就是这个原始路径，重启加载它
	// 即可拿到新版本。
	binaryPath, _ := os.Executable()

	result, err := h.updateService.Apply(r.Context())
	if err != nil {
		if errors.Is(err, upgrade.ErrNoUpdate) {
			respondJSON(w, http.StatusOK, H{
				"status": "no_update",
				"result": result,
			})
			return
		}
		if errors.Is(err, service.ErrUpdateInProgress) {
			respondJSON(w, http.StatusConflict, H{
				"status": "in_progress",
				"error":  "更新正在进行中，请稍候",
			})
			return
		}
		respondJSON(w, http.StatusInternalServerError, H{"error": err.Error()})
		return
	}

	respondJSON(w, http.StatusOK, H{
		"status":  "updated",
		"restart": req.Restart,
		"result":  result,
	})

	service.NewWebhookService().HandleDataEvent(service.WebhookEventSystemUpdateCompleted, H{
		"status":           "updated",
		"current_version":  result.CurrentVersion,
		"latest_version":   result.LatestVersion,
		"package_checksum": result.PackageChecksum,
		"restart":          req.Restart,
	})

	if req.Restart {
		restartLater(binaryPath)
	}
}

// Restart 重启当前应用
func (h *UpdateHandler) Restart(w http.ResponseWriter, r *http.Request) {
	setNoCache(w)
	respondJSON(w, http.StatusOK, H{"status": "restarting"})
	restartLater("")
}

// restartLater 延迟重启当前进程。
//
// targetPath 用于显式指定重启加载的二进制路径：更新流程会把运行中的二进制
// 重命名为 backup，导致 os.Executable 漂移到旧版本路径，因此更新后重启必须
// 传入更新前捕获的路径；为空时回退到 os.Executable（适用于未发生重命名的
// 普通重启场景）。
func restartLater(targetPath string) {
	go func() {
		time.Sleep(800 * time.Millisecond)
		if err := restartAt(targetPath); err != nil {
			log.Printf("restart failed: %v", err)
		}
	}()
}

// restartAt 通过 syscall.Exec 重启进程。优先使用 targetPath 指定的二进制，
// 为空时回退到 os.Executable。
func restartAt(targetPath string) error {
	self := restartTargetPath(targetPath)
	if self == "" {
		return errors.New("unable to resolve restart binary path")
	}

	if runtime.GOOS == "windows" {
		cmd := exec.Command(self, os.Args[1:]...)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		cmd.Stdin = os.Stdin
		cmd.Env = os.Environ()
		if err := cmd.Start(); err != nil {
			return err
		}
		os.Exit(0)
	}

	return syscall.Exec(self, os.Args, os.Environ())
}

// restartTargetPath 计算重启时应加载的二进制路径：优先使用显式传入的路径，
// 否则回退到 os.Executable。
func restartTargetPath(targetPath string) string {
	if targetPath != "" {
		return targetPath
	}
	if exe, err := os.Executable(); err == nil {
		return exe
	}
	return ""
}
