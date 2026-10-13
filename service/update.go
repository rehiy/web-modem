package service

import (
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/rehiy/libgo/upgrade"
	"github.com/rehiy/web-modem/appinfo"
)

const (
	defaultGitHubAPIBase = "https://api.github.com"

	updateRequestAttempts = 3
	updateRetryBaseDelay  = 500 * time.Millisecond
)

// ErrUpdateInProgress 表示已有更新任务正在执行。
var ErrUpdateInProgress = errors.New("update already in progress")

type UpdateService struct {
	client  *http.Client
	applyMu sync.Mutex
}

type UpdateCheckResult struct {
	CurrentVersion  string              `json:"current_version"`
	LatestVersion   string              `json:"latest_version"`
	HasUpdate       bool                `json:"has_update"`
	PackageChecksum string              `json:"package_checksum,omitempty"`
	Info            *upgrade.UpdateInfo `json:"info"`
}

type githubRelease struct {
	TagName string        `json:"tag_name"`
	Name    string        `json:"name"`
	Body    string        `json:"body"`
	Assets  []githubAsset `json:"assets"`
}

type githubAsset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
	Digest             string `json:"digest"`
	Size               int64  `json:"size"`
}

func NewUpdateService() *UpdateService {
	return &UpdateService{
		client: &http.Client{Timeout: 10 * time.Minute},
	}
}

func (s *UpdateService) Check(ctx context.Context) (*UpdateCheckResult, error) {
	release, err := s.fetchLatestRelease(ctx)
	if err != nil {
		return nil, err
	}

	info := &upgrade.UpdateInfo{
		Type:    "github",
		Message: "已是最新版本",
		Release: release.Body,
		Version: release.TagName,
	}

	result := &UpdateCheckResult{
		CurrentVersion: appinfo.Version,
		LatestVersion:  release.TagName,
		Info:           info,
	}

	if compareVersion(release.TagName, appinfo.Version) <= 0 {
		return result, nil
	}

	asset, ok := findRuntimeAsset(release.Assets)
	if !ok {
		return nil, fmt.Errorf("未找到当前平台的更新包: %s/%s", runtime.GOOS, runtime.GOARCH)
	}

	info.Message = "发现新版本"
	info.Package = asset.BrowserDownloadURL
	result.PackageChecksum, err = s.fetchPackageChecksum(ctx, release, asset)
	if err != nil {
		return nil, err
	}
	result.HasUpdate = true

	return result, nil
}

func (s *UpdateService) Apply(ctx context.Context) (*UpdateCheckResult, error) {
	if !s.applyMu.TryLock() {
		return nil, ErrUpdateInProgress
	}
	defer s.applyMu.Unlock()

	result, err := s.Check(ctx)
	if err != nil {
		return nil, err
	}
	if !result.HasUpdate {
		return result, upgrade.ErrNoUpdate
	}

	server := newStaticUpdateServer(result.Info)
	defer server.Close()

	updater := upgrade.NewUpdater(server.URL, appinfo.Version)
	updater.Download = s.downloadPackage(ctx, result.PackageChecksum)
	if err := updater.Apply(); err != nil {
		return result, err
	}

	return result, nil
}

func newStaticUpdateServer(info *upgrade.UpdateInfo) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(info)
	}))
}

func (s *UpdateService) fetchLatestRelease(ctx context.Context) (*githubRelease, error) {
	repo := appinfo.UpdateRepo
	if repo == "" {
		return nil, errors.New("未配置更新仓库 (UPDATE_REPO)")
	}

	reqURL := fmt.Sprintf("%s/repos/%s/releases/latest", defaultGitHubAPIBase, repo)
	resp, err := s.doRequest(ctx, reqURL, "application/vnd.github+json")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return nil, fmt.Errorf("查询最新版本失败: %s %s", resp.Status, strings.TrimSpace(string(body)))
	}

	var release githubRelease
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return nil, err
	}
	if release.TagName == "" {
		return nil, errors.New("最新版本响应缺少 tag_name")
	}

	return &release, nil
}

// GitHub 旧版资产没有 digest，使用发布流程附带的校验清单。
func (s *UpdateService) fetchPackageChecksum(ctx context.Context, release *githubRelease, asset githubAsset) (string, error) {
	if strings.HasPrefix(asset.Digest, "sha256:") {
		return strings.TrimPrefix(asset.Digest, "sha256:"), nil
	}
	for _, candidate := range release.Assets {
		if candidate.Name != "checksums.txt" {
			continue
		}
		resp, err := s.doRequest(ctx, candidate.BrowserDownloadURL, "application/octet-stream")
		if err != nil {
			return "", err
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return "", fmt.Errorf("下载校验清单失败: %s", resp.Status)
		}
		data, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		if err != nil {
			return "", err
		}
		for _, line := range strings.Split(string(data), "\n") {
			fields := strings.Fields(line)
			if len(fields) == 2 && strings.TrimPrefix(fields[1], "*") == asset.Name {
				return fields[0], nil
			}
		}
		return "", fmt.Errorf("校验清单缺少更新包: %s", asset.Name)
	}
	return "", nil
}

func (s *UpdateService) downloadPackage(ctx context.Context, packageChecksum string) upgrade.DownloadHandler {
	return func(pkgURL, outputPath string) (string, error) {
		var lastErr error
		for attempt := 1; attempt <= updateRequestAttempts; attempt++ {
			output, err := s.downloadPackageOnce(ctx, pkgURL, outputPath, packageChecksum)
			if err == nil {
				return output, nil
			}
			if ctx.Err() != nil {
				return "", ctx.Err()
			}
			lastErr = err
			if attempt == updateRequestAttempts {
				break
			}
			if err := waitUpdateRetry(ctx, attempt); err != nil {
				return "", err
			}
		}

		return "", fmt.Errorf("下载更新包失败（已重试 %d 次）: %w", updateRequestAttempts, lastErr)
	}
}

// downloadPackageOnce 下载并校验一次更新包，失败由上层统一重试。
func (s *UpdateService) downloadPackageOnce(ctx context.Context, pkgURL, outputPath, packageChecksum string) (string, error) {
	resp, err := s.doRequest(ctx, pkgURL, "application/octet-stream")
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return "", fmt.Errorf("下载更新包失败: %s %s", resp.Status, strings.TrimSpace(string(body)))
	}

	var packageHash = sha256.New()
	reader := io.Reader(resp.Body)
	if packageChecksum != "" {
		reader = io.TeeReader(resp.Body, packageHash)
	}

	if isGzipPackage(pkgURL, resp.Header.Get("Content-Type")) {
		gzReader, err := gzip.NewReader(reader)
		if err != nil {
			return "", err
		}
		defer gzReader.Close()
		reader = gzReader
	}

	tmpPath := outputPath + ".download"
	file, err := os.Create(tmpPath)
	if err != nil {
		return "", err
	}
	if _, err = io.Copy(file, reader); err != nil {
		file.Close()
		_ = os.Remove(tmpPath)
		return "", err
	}
	if packageChecksum != "" && !strings.EqualFold(fmt.Sprintf("%x", packageHash.Sum(nil)), packageChecksum) {
		file.Close()
		_ = os.Remove(tmpPath)
		return "", errors.New("更新包 SHA256 校验失败")
	}
	if err = file.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return "", err
	}
	if err = os.Rename(tmpPath, outputPath); err != nil {
		_ = os.Remove(tmpPath)
		return "", err
	}

	return outputPath, nil
}

// doRequest 对临时网络错误和服务端错误做有限次数重试，避免一次抖动直接终止更新。
func (s *UpdateService) doRequest(ctx context.Context, reqURL, accept string) (*http.Response, error) {
	client := s.client
	if client == nil {
		client = http.DefaultClient
	}

	var lastErr error
	for attempt := 1; attempt <= updateRequestAttempts; attempt++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("Accept", accept)
		req.Header.Set("Accept-Encoding", "identity")
		req.Header.Set("User-Agent", appinfo.Name)

		resp, err := client.Do(req)
		if err == nil {
			if attempt == updateRequestAttempts || !isRetryableUpdateStatus(resp.StatusCode) {
				return resp, nil
			}
			_, _ = io.Copy(io.Discard, resp.Body)
			_ = resp.Body.Close()
			lastErr = fmt.Errorf("GitHub 请求返回 %s", resp.Status)
		} else {
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			lastErr = err
			if attempt == updateRequestAttempts {
				return nil, err
			}
		}

		if err := waitUpdateRetry(ctx, attempt); err != nil {
			return nil, err
		}
	}

	return nil, lastErr
}

func isRetryableUpdateStatus(status int) bool {
	return status == http.StatusRequestTimeout || status == http.StatusTooManyRequests || status >= http.StatusInternalServerError
}

func waitUpdateRetry(ctx context.Context, attempt int) error {
	timer := time.NewTimer(time.Duration(attempt) * updateRetryBaseDelay)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func findRuntimeAsset(assets []githubAsset) (githubAsset, bool) {
	name := fmt.Sprintf("%s-%s-%s", appinfo.Name, runtime.GOOS, runtime.GOARCH)
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	name += ".gz"

	for _, asset := range assets {
		if asset.Name == name {
			return asset, true
		}
	}
	return githubAsset{}, false
}

func isGzipPackage(pkgURL, contentType string) bool {
	parsed, err := url.Parse(pkgURL)
	if err == nil && strings.HasSuffix(parsed.Path, ".gz") {
		return true
	}
	return strings.Contains(strings.ToLower(contentType), "gzip")
}

func compareVersion(latest, current string) int {
	if latest == current {
		return 0
	}

	latestParts := parseVersionParts(latest)
	currentParts := parseVersionParts(current)
	for i := 0; i < len(latestParts) || i < len(currentParts); i++ {
		var latestPart, currentPart int
		if i < len(latestParts) {
			latestPart = latestParts[i]
		}
		if i < len(currentParts) {
			currentPart = currentParts[i]
		}

		if latestPart > currentPart {
			return 1
		}
		if latestPart < currentPart {
			return -1
		}
	}

	// 数字部分相同但标签前缀不同（例如 v1.2.3 与 1.2.3）时视为同一版本。
	if normalizeVersionTag(latest) == normalizeVersionTag(current) {
		return 0
	}
	// 对非标准版本保留字符串比较，避免完全无法解析的标签被静默视为相同。
	return strings.Compare(latest, current)
}

func normalizeVersionTag(version string) string {
	version = strings.TrimSpace(version)
	version = strings.TrimPrefix(version, "v")
	version = strings.TrimPrefix(version, "V")
	return strings.Split(version, "-")[0]
}

func parseVersionParts(version string) []int {
	version = normalizeVersionTag(version)

	parts := strings.Split(version, ".")
	values := make([]int, 0, len(parts))
	for _, part := range parts {
		value, err := strconv.Atoi(part)
		if err != nil {
			return []int{0}
		}
		values = append(values, value)
	}
	return values
}
