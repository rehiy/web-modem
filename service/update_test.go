package service

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/rehiy/libgo/upgrade"
	"github.com/rehiy/web-modem/appinfo"
)

type updateRoundTripFunc func(*http.Request) (*http.Response, error)

func (f updateRoundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}

func updateTestResponse(status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Status:     fmt.Sprintf("%d test", status),
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}

type updateErrorReader struct{}

func (updateErrorReader) Read([]byte) (int, error) {
	return 0, errors.New("temporary download failure")
}

func (updateErrorReader) Close() error {
	return nil
}

func TestDoRequestRetriesTransientStatus(t *testing.T) {
	attempts := 0
	service := &UpdateService{client: &http.Client{
		Transport: updateRoundTripFunc(func(_ *http.Request) (*http.Response, error) {
			attempts++
			if attempts < updateRequestAttempts {
				return updateTestResponse(http.StatusServiceUnavailable, "try again"), nil
			}
			return updateTestResponse(http.StatusOK, "ok"), nil
		}),
	}}

	response, err := service.doRequest(context.Background(), "https://example.test/update", "application/json")
	if err != nil {
		t.Fatalf("doRequest returned error: %v", err)
	}
	defer response.Body.Close()

	if attempts != updateRequestAttempts {
		t.Fatalf("expected %d attempts, got %d", updateRequestAttempts, attempts)
	}
	if response.StatusCode != http.StatusOK {
		t.Fatalf("expected final status 200, got %d", response.StatusCode)
	}
}

func TestDoRequestStopsRetryingNonTransientStatus(t *testing.T) {
	attempts := 0
	service := &UpdateService{client: &http.Client{
		Transport: updateRoundTripFunc(func(_ *http.Request) (*http.Response, error) {
			attempts++
			return updateTestResponse(http.StatusNotFound, "missing"), nil
		}),
	}}

	response, err := service.doRequest(context.Background(), "https://example.test/update", "application/json")
	if err != nil {
		t.Fatalf("doRequest returned error: %v", err)
	}
	defer response.Body.Close()

	if attempts != 1 {
		t.Fatalf("non-transient status should not be retried, got %d attempts", attempts)
	}
	if response.StatusCode != http.StatusNotFound {
		t.Fatalf("expected final status 404, got %d", response.StatusCode)
	}
}

func TestUpdateServiceRejectsConcurrentApply(t *testing.T) {
	service := NewUpdateService()
	if !service.applyMu.TryLock() {
		t.Fatal("failed to prepare update lock")
	}
	defer service.applyMu.Unlock()

	if _, err := service.Apply(context.Background()); err != ErrUpdateInProgress {
		t.Fatalf("expected ErrUpdateInProgress, got %v", err)
	}
}

func TestDownloadPackageRetriesReadFailure(t *testing.T) {
	attempts := 0
	service := &UpdateService{client: &http.Client{
		Transport: updateRoundTripFunc(func(_ *http.Request) (*http.Response, error) {
			attempts++
			return &http.Response{
				StatusCode: http.StatusOK,
				Status:     "200 test",
				Header:     make(http.Header),
				Body:       updateErrorReader{},
			}, nil
		}),
	}}

	outputPath := t.TempDir() + "/web-modem"
	output, err := service.downloadPackage(context.Background(), "")("https://example.test/web-modem", outputPath)
	if err == nil || output != "" {
		t.Fatalf("expected download failure, output=%q err=%v", output, err)
	}
	if attempts != updateRequestAttempts {
		t.Fatalf("expected %d download retries, got %d requests", updateRequestAttempts, attempts)
	}
}

func TestCompareVersionNormalizesPrefix(t *testing.T) {
	if got := compareVersion("v1.2.3", "1.2.3"); got != 0 {
		t.Fatalf("expected equal numeric versions to compare equal, got %d", got)
	}
	if got := compareVersion("v1.2.4", "1.2.3"); got != 1 {
		t.Fatalf("expected v1.2.4 to be newer, got %d", got)
	}
}

func TestCheckGitHubRelease(t *testing.T) {
	assetName := fmt.Sprintf("%s-%s-%s", appinfo.Name, runtime.GOOS, runtime.GOARCH)
	if runtime.GOOS == "windows" {
		assetName += ".exe"
	}
	assetName += ".gz"
	checksum := strings.Repeat("a", 64)
	for _, useDigest := range []bool{true, false} {
		t.Run(fmt.Sprintf("digest=%v", useDigest), func(t *testing.T) {
			asset := githubAsset{
				Name: assetName, BrowserDownloadURL: "https://github.com/rehiy/web-modem/releases/download/v99.0.0/" + assetName,
			}
			if useDigest {
				asset.Digest = "sha256:" + checksum
			}
			release := githubRelease{TagName: "v99.0.0", Body: "release notes", Assets: []githubAsset{
				{Name: "wrong-platform.gz", Digest: "sha256:wrong"}, asset,
				{Name: "checksums.txt", BrowserDownloadURL: "https://example.test/checksums.txt"},
			}}
			service := &UpdateService{client: &http.Client{Transport: updateRoundTripFunc(func(r *http.Request) (*http.Response, error) {
				if r.Header.Get("Authorization") != "" {
					t.Error("公开 GitHub 更新不应携带 Token")
				}
				switch r.URL.String() {
				case defaultGitHubAPIBase + "/repos/rehiy/web-modem/releases/latest":
					if r.Header.Get("Accept") != "application/vnd.github+json" {
						t.Error("错误的 GitHub API Accept")
					}
					data, _ := json.Marshal(release)
					return updateTestResponse(http.StatusOK, string(data)), nil
				case "https://example.test/checksums.txt":
					if useDigest {
						t.Error("有 digest 时不应请求校验清单")
					}
					return updateTestResponse(http.StatusOK, "wrong  other.gz\n"+checksum+"  "+assetName+"\n"), nil
				default:
					t.Errorf("意外更新请求: %s", r.URL)
					return updateTestResponse(http.StatusNotFound, "missing"), nil
				}
			})}}
			result, err := service.Check(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			if !result.HasUpdate || result.Info.Type != "github" || result.Info.Package != asset.BrowserDownloadURL || result.PackageChecksum != checksum {
				t.Fatalf("GitHub 更新包或校验和错误: %+v", result)
			}
		})
	}
}

func TestGitHubChecksumManifestMissingAsset(t *testing.T) {
	service := &UpdateService{client: &http.Client{Transport: updateRoundTripFunc(func(r *http.Request) (*http.Response, error) {
		return updateTestResponse(http.StatusOK, "hash  other.gz\n"), nil
	})}}
	release := &githubRelease{Assets: []githubAsset{{Name: "checksums.txt", BrowserDownloadURL: "https://example.test/checksums.txt"}}}
	if _, err := service.fetchPackageChecksum(context.Background(), release, githubAsset{Name: "web-modem.gz"}); err == nil {
		t.Fatal("存在校验清单但缺少当前包时不能跳过校验")
	}
}

func TestDownloadPackageVerifiesArchiveBeforeReplacingBinary(t *testing.T) {
	var archive bytes.Buffer
	writer := gzip.NewWriter(&archive)
	_, _ = writer.Write([]byte("new binary"))
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	checksum := fmt.Sprintf("%x", sha256.Sum256(archive.Bytes()))
	service := &UpdateService{client: &http.Client{Transport: updateRoundTripFunc(func(r *http.Request) (*http.Response, error) {
		return updateTestResponse(http.StatusOK, archive.String()), nil
	})}}
	output := filepath.Join(t.TempDir(), "web-modem")
	if err := os.WriteFile(output, []byte("old binary"), 0755); err != nil {
		t.Fatal(err)
	}
	if _, err := service.downloadPackageOnce(context.Background(), "https://example.test/app.gz", output, strings.Repeat("0", 64)); err == nil {
		t.Fatal("错误校验和应阻止覆盖二进制")
	}
	if data, _ := os.ReadFile(output); string(data) != "old binary" {
		t.Fatalf("校验失败损坏旧二进制: %q", data)
	}
	if _, err := os.Stat(output + ".download"); !os.IsNotExist(err) {
		t.Fatalf("校验失败残留临时文件: %v", err)
	}
	newPath := output + ".new"
	if _, err := service.downloadPackageOnce(context.Background(), "https://example.test/app.gz", newPath, checksum); err != nil {
		t.Fatal(err)
	}
	updater := &upgrade.BinaryUpdater{NewVersion: "v2", OldVersion: "v1", TargetPath: output, NewBinary: newPath}
	if err := updater.Init(); err != nil {
		t.Fatal(err)
	}
	if err := updater.CommitBinary(); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(output)
	if err != nil || string(data) != "new binary" {
		t.Fatalf("上游 Libgo 未正确替换二进制: %q, %v", data, err)
	}
	info, err := os.Stat(output)
	if err != nil || info.Mode().Perm() != 0755 {
		t.Fatalf("上游 Libgo 未保留执行权限: %v", err)
	}
}
