package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// TestRunUpdateAppliesBinaryAndMissingConfig 验证单一 update 命令的主路径：
// 检测到新版本后，在 --yes 确认模式下替换指定二进制，并只追加缺失的 Claude 配置。
func TestRunUpdateAppliesBinaryAndMissingConfig(t *testing.T) {
	oldVersion, oldCommit, oldDate := version, commit, date
	oldDo := httpClientDo
	t.Cleanup(func() {
		version, commit, date = oldVersion, oldCommit, oldDate
		httpClientDo = oldDo
	})
	version, commit, date = "v0.1.0", "test", "test"

	dir := t.TempDir()
	binaryPath := filepath.Join(dir, "go-review")
	writeFile(t, binaryPath, "old-binary")
	if err := os.Chmod(binaryPath, 0o755); err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(dir, ".go-review", "go-review.yaml")
	writeFile(t, configPath, oldConfigWithoutClaude())

	archiveName := releaseArchiveName("v0.2.0", runtimeGOOS(), runtimeGOARCH())
	archiveData := makeReleaseArchive(t, "go-review", "new-binary")
	checksums := fmt.Sprintf("%x  %s\n", sha256.Sum256(archiveData), archiveName)
	httpClientDo = fakeHTTP(map[string]string{
		"https://example.test/latest":    fmt.Sprintf(`{"tag_name":"v0.2.0","name":"v0.2.0","body":"- 新增 Claude 复盘","assets":[{"name":%q,"browser_download_url":"https://example.test/archive"},{"name":"checksums.txt","browser_download_url":"https://example.test/checksums"}]}`, archiveName),
		"https://example.test/archive":   string(archiveData),
		"https://example.test/checksums": checksums,
	})

	stdout, stderr, code := captureRun([]string{"update", "--yes", "--release-url", "https://example.test/latest", "--binary", binaryPath, "--config", configPath})
	if code != 0 {
		t.Fatalf("update code=%d stderr=%q stdout=%q", code, stderr, stdout)
	}
	for _, want := range []string{"发现新版本", "UPDATED go-review v0.1.0 -> v0.2.0", "adapters[id=llm.claude]", "steps[id=llm-claude]", "profiles[review].steps += llm-claude"} {
		if !strings.Contains(stdout, want) {
			t.Fatalf("stdout missing %q:\n%s", want, stdout)
		}
	}
	binaryData, err := os.ReadFile(binaryPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(binaryData) != "new-binary" {
		t.Fatalf("binary data = %q", binaryData)
	}
	if _, err := os.Stat(binaryPath + ".bak-v0.1.0"); err != nil {
		t.Fatalf("expected backup: %v", err)
	}
	configData, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"llm.claude: \"claude\"", "- id: llm.claude", "- id: llm-claude", "steps: [format-check, lint, test, semantic, llm-review, llm-claude]"} {
		if !strings.Contains(string(configData), want) {
			t.Fatalf("config missing %q:\n%s", want, configData)
		}
	}
}

// TestRunUpdateCancelDoesNotModifyFiles 验证未确认升级时只展示检测结果，不替换二进制、不改配置。
func TestRunUpdateCancelDoesNotModifyFiles(t *testing.T) {
	oldVersion, oldCommit, oldDate := version, commit, date
	oldDo := httpClientDo
	t.Cleanup(func() {
		version, commit, date = oldVersion, oldCommit, oldDate
		httpClientDo = oldDo
	})
	version, commit, date = "v0.1.0", "test", "test"

	dir := t.TempDir()
	binaryPath := filepath.Join(dir, "go-review")
	writeFile(t, binaryPath, "old-binary")
	configPath := filepath.Join(dir, ".go-review", "go-review.yaml")
	writeFile(t, configPath, oldConfigWithoutClaude())
	archiveName := releaseArchiveName("v0.2.0", runtimeGOOS(), runtimeGOARCH())
	httpClientDo = fakeHTTP(map[string]string{
		"https://example.test/latest": fmt.Sprintf(`{"tag_name":"v0.2.0","assets":[{"name":%q,"browser_download_url":"https://example.test/archive"},{"name":"checksums.txt","browser_download_url":"https://example.test/checksums"}]}`, archiveName),
	})

	stdin := os.Stdin
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	_, _ = w.WriteString("n\n")
	_ = w.Close()
	os.Stdin = r
	t.Cleanup(func() { os.Stdin = stdin })

	stdout, stderr, code := captureRun([]string{"update", "--release-url", "https://example.test/latest", "--binary", binaryPath, "--config", configPath})
	if code != 0 {
		t.Fatalf("update cancel code=%d stderr=%q stdout=%q", code, stderr, stdout)
	}
	if !strings.Contains(stdout, "已取消升级") {
		t.Fatalf("stdout missing cancel message:\n%s", stdout)
	}
	binaryData, err := os.ReadFile(binaryPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(binaryData) != "old-binary" {
		t.Fatalf("binary should not change: %q", binaryData)
	}
	configData, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(configData), "llm.claude") {
		t.Fatalf("config should not change:\n%s", configData)
	}
}

func fakeHTTP(responses map[string]string) func(*http.Request) (*http.Response, error) {
	return func(req *http.Request) (*http.Response, error) {
		body, ok := responses[req.URL.String()]
		if !ok {
			return &http.Response{StatusCode: 404, Status: "404 Not Found", Body: io.NopCloser(strings.NewReader("not found")), Header: make(http.Header)}, nil
		}
		return &http.Response{StatusCode: 200, Status: "200 OK", Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}, nil
	}
}

func makeReleaseArchive(t *testing.T, name string, content string) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	data := []byte(content)
	if err := tw.WriteHeader(&tar.Header{Name: "go-review_v0.2.0_test/" + name, Mode: 0o755, Size: int64(len(data))}); err != nil {
		t.Fatal(err)
	}
	if _, err := tw.Write(data); err != nil {
		t.Fatal(err)
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func oldConfigWithoutClaude() string {
	return `schema_version: "1.0"
tools:
  go_review: "generated"
  adapters:
    go.lint: "system-golangci-lint"
    llm.review: "codex"
defaults:
  workdir: .
adapters:
  - id: go.lint.format
    type: go.lint
    fix_safety: safe
  - id: go.lint.static
    type: go.lint
  - id: go.test
    type: cmd
  - id: go.semantic
    type: go.semantic
  - id: llm.review
    type: llm.review
    capabilities: [report]
    fix_safety: review
steps:
  - id: format-check
    adapter: go.lint.format
    allow_fix: true
  - id: lint
    adapter: go.lint.static
  - id: test
    adapter: go.test
  - id: semantic
    adapter: go.semantic
  - id: llm-review
    adapter: llm.review
    enabled: false
profiles:
  - name: review
    steps: [format-check, lint, test, semantic, llm-review]
  - name: fast
    steps: [format-check]
`
}

func runtimeGOOS() string   { return runtime.GOOS }
func runtimeGOARCH() string { return runtime.GOARCH }
