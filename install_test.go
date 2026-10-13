package main

import (
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// 用本地 GitHub Releases 响应验证完整安装流程，安装路径全部位于临时目录。
func TestInstallFromGitHubRelease(t *testing.T) {
	if runtime.GOOS != "linux" && runtime.GOOS != "darwin" {
		t.Skip("安装脚本仅支持 Linux/macOS")
	}
	for _, name := range []string{"sh", "curl", "gzip", "install"} {
		if _, err := exec.LookPath(name); err != nil {
			t.Skipf("安装测试缺少 %s", name)
		}
	}
	var archive bytes.Buffer
	writer := gzip.NewWriter(&archive)
	_, _ = writer.Write([]byte("fixture binary"))
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	assetName := fmt.Sprintf("web-modem-%s-%s.gz", runtime.GOOS, runtime.GOARCH)
	checksum := fmt.Sprintf("%x", sha256.Sum256(archive.Bytes()))
	for _, mode := range []string{"digest", "manifest", "bad-checksum", "preserve-config", "no-python-digest", "no-python-manifest"} {
		t.Run(mode, func(t *testing.T) {
			server := httptest.NewUnstartedServer(nil)
			serverURL := "http://" + server.Listener.Addr().String()
			server.Config.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Header.Get("Authorization") != "" {
					t.Error("公开安装不应携带 Token")
				}
				switch r.URL.Path {
				case "/repos/rehiy/web-modem/releases/latest":
					digest := "sha256:" + checksum
					if mode == "manifest" || mode == "no-python-manifest" {
						digest = ""
					}
					if mode == "bad-checksum" {
						digest = "sha256:" + strings.Repeat("0", 64)
					}
					_ = json.NewEncoder(w).Encode(map[string]any{
						"tag_name": "v1.2.3", "assets": []map[string]any{
							{"name": assetName, "url": serverURL + "/asset-api", "browser_download_url": serverURL + "/" + assetName, "digest": digest},
							{"name": "checksums.txt", "browser_download_url": serverURL + "/checksums.txt"},
						},
					})
				case "/" + assetName:
					_, _ = w.Write(archive.Bytes())
				case "/checksums.txt":
					_, _ = fmt.Fprintf(w, "%s  %s\n", checksum, assetName)
				default:
					t.Errorf("安装请求了错误地址: %s", r.URL)
					http.NotFound(w, r)
				}
			})
			server.Start()
			defer server.Close()
			root := t.TempDir()
			if err := os.Chmod(root, 0700); err != nil {
				t.Fatal(err)
			}
			configPath := filepath.Join(root, "modem.env")
			if mode == "preserve-config" {
				if err := os.WriteFile(configPath, []byte("HTTP_PORT=18080\n"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			cmd := exec.Command("sh", "install.sh")
			cmd.Env = append(os.Environ(),
				"GITHUB_API_BASE="+server.URL, "GITHUB_REPO=rehiy/web-modem", "INSTALL_SERVICE=0", "AUTH_MODE=off",
				"INSTALL_DIR="+filepath.Join(root, "bin"), "ENV_FILE="+configPath, "DB_PATH="+filepath.Join(root, "data", "modem.db"),
			)
			if strings.HasPrefix(mode, "no-python-") {
				binDir := filepath.Join(root, "tools")
				if err := os.Mkdir(binDir, 0755); err != nil {
					t.Fatal(err)
				}
				for _, name := range []string{"curl", "gzip", "install", "uname", "id", "tr", "sed", "grep", "head", "awk", "mktemp", "rm", "dirname", "chmod", "cat", "sha256sum", "shasum"} {
					if path, err := exec.LookPath(name); err == nil {
						if err := os.Symlink(path, filepath.Join(binDir, name)); err != nil {
							t.Fatal(err)
						}
					}
				}
				cmd.Env = append(cmd.Env, "PATH="+binDir)
			}
			output, err := cmd.CombinedOutput()
			binary := filepath.Join(root, "bin", "web-modem")
			if mode == "bad-checksum" {
				if err == nil || !strings.Contains(string(output), "SHA256 校验失败") {
					t.Fatalf("安装未拒绝错误校验和: %v\n%s", err, output)
				}
				if _, err := os.Stat(binary); !os.IsNotExist(err) {
					t.Fatalf("校验失败仍安装了二进制: %v", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("安装失败: %v\n%s", err, output)
			}
			data, err := os.ReadFile(binary)
			if err != nil || string(data) != "fixture binary" {
				t.Fatalf("安装二进制错误: %q, %v", data, err)
			}
			config, err := os.ReadFile(configPath)
			if err != nil {
				t.Fatal(err)
			}
			for path, want := range map[string]os.FileMode{root: 0700, configPath: 0600, binary: 0755} {
				info, err := os.Stat(path)
				if err != nil {
					t.Fatal(err)
				}
				if got := info.Mode().Perm(); got != want {
					t.Errorf("安装改变了 %s 的权限: got %o, want %o", path, got, want)
				}
			}
			if mode == "preserve-config" && string(config) != "HTTP_PORT=18080\n" {
				t.Fatalf("安装覆盖了现有配置: %q", config)
			}
		})
	}
}
