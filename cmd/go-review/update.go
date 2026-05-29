package main

import (
	"archive/tar"
	"archive/zip"
	"bufio"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/v111nce/go-review/internal/config"
)

const defaultLatestReleaseURL = "https://api.github.com/repos/v111nce/go-review/releases/latest"

// httpClientDo 是 update 命令的网络边界。测试会替换它，生产运行默认使用 http.DefaultClient。
var httpClientDo = http.DefaultClient.Do

// githubRelease 是 GitHub Releases latest API 中 go-review 需要的最小字段集合。
type githubRelease struct {
	TagName string         `json:"tag_name"`
	Name    string         `json:"name"`
	Body    string         `json:"body"`
	Assets  []releaseAsset `json:"assets"`
}

type releaseAsset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
}

type updatePlan struct {
	CurrentVersion string
	LatestVersion  string
	ReleaseName    string
	ReleaseBody    string
	Archive        releaseAsset
	Checksums      releaseAsset
}

type updateResult struct {
	OldVersion    string
	NewVersion    string
	BinaryPath    string
	BackupPath    string
	ConfigPath    string
	ConfigAdded   []string
	ConfigSkipped []string
}

// runUpdate 执行单一 update 命令：先检测新版，展示概要并询问；用户确认后才替换二进制并合并缺失配置。
func runUpdate(args []string) int {
	fs := flag.NewFlagSet("update", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	configPath := fs.String("config", "", "path to go-review YAML config; defaults to discovered project config")
	workdir := fs.String("workdir", "", "project working directory override for config discovery")
	releaseURL := fs.String("release-url", defaultLatestReleaseURL, "latest release API URL")
	binaryPath := fs.String("binary", "", "path to go-review executable to replace; defaults to current executable")
	yes := fs.Bool("yes", false, "upgrade without interactive confirmation")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() != 0 {
		fmt.Fprintf(os.Stderr, "go-review update: unexpected argument %q\n", fs.Arg(0))
		return 2
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	currentVersion, _, _ := buildMetadata()
	plan, err := buildUpdatePlan(ctx, *releaseURL, currentVersion)
	if err != nil {
		fmt.Fprintf(os.Stderr, "go-review update: %v\n", err)
		return 2
	}
	if plan == nil {
		fmt.Fprintf(os.Stdout, "当前已是最新版本：%s\n", currentVersion)
		return 0
	}

	printUpdateAvailable(plan, os.Stdout)
	if !*yes {
		ok, err := confirmUpdate(os.Stdin, os.Stdout)
		if err != nil {
			fmt.Fprintf(os.Stderr, "go-review update: %v\n", err)
			return 2
		}
		if !ok {
			fmt.Fprintln(os.Stdout, "已取消升级，未修改二进制或配置。")
			return 0
		}
	}

	resolvedConfigPath, configExplicit, err := resolveUpdateConfigPath(*configPath, *workdir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "go-review update: %v\n", err)
		return 2
	}
	resolvedBinaryPath := *binaryPath
	if strings.TrimSpace(resolvedBinaryPath) == "" {
		resolvedBinaryPath, err = os.Executable()
		if err != nil {
			fmt.Fprintf(os.Stderr, "go-review update: resolve executable: %v\n", err)
			return 2
		}
	}

	result, err := applyUpdate(ctx, plan, resolvedBinaryPath, resolvedConfigPath, configExplicit)
	if err != nil {
		fmt.Fprintf(os.Stderr, "go-review update: %v\n", err)
		return 2
	}
	printUpdateResult(result, os.Stdout)
	return 0
}

// buildUpdatePlan 下载 latest release 元数据并判断当前版本是否落后。
func buildUpdatePlan(ctx context.Context, latestReleaseURL string, currentVersion string) (*updatePlan, error) {
	var release githubRelease
	if err := getJSON(ctx, latestReleaseURL, &release); err != nil {
		return nil, err
	}
	latest := strings.TrimSpace(release.TagName)
	if latest == "" {
		return nil, errors.New("latest release missing tag_name")
	}
	cmp, err := compareVersions(currentVersion, latest)
	if err != nil {
		return nil, err
	}
	if cmp >= 0 {
		return nil, nil
	}
	archiveName := releaseArchiveName(latest, runtime.GOOS, runtime.GOARCH)
	archive, ok := findReleaseAsset(release.Assets, archiveName)
	if !ok {
		return nil, fmt.Errorf("release %s missing asset %s", latest, archiveName)
	}
	checksums, ok := findReleaseAsset(release.Assets, "checksums.txt")
	if !ok {
		return nil, fmt.Errorf("release %s missing checksums.txt", latest)
	}
	return &updatePlan{
		CurrentVersion: currentVersion,
		LatestVersion:  latest,
		ReleaseName:    release.Name,
		ReleaseBody:    release.Body,
		Archive:        archive,
		Checksums:      checksums,
	}, nil
}

// applyUpdate 下载、校验、替换二进制，并对项目配置执行“只追加缺失项”的升级。
func applyUpdate(ctx context.Context, plan *updatePlan, binaryPath string, configPath string, configExplicit bool) (updateResult, error) {
	archiveData, err := getBytes(ctx, plan.Archive.BrowserDownloadURL)
	if err != nil {
		return updateResult{}, fmt.Errorf("download %s: %w", plan.Archive.Name, err)
	}
	checksumsData, err := getBytes(ctx, plan.Checksums.BrowserDownloadURL)
	if err != nil {
		return updateResult{}, fmt.Errorf("download checksums: %w", err)
	}
	if err := verifyChecksum(plan.Archive.Name, archiveData, string(checksumsData)); err != nil {
		return updateResult{}, err
	}

	tmpDir, err := os.MkdirTemp("", "go-review-update-*")
	if err != nil {
		return updateResult{}, err
	}
	defer os.RemoveAll(tmpDir)
	newBinary, err := extractGoReviewBinary(tmpDir, plan.Archive.Name, archiveData)
	if err != nil {
		return updateResult{}, err
	}
	backup, err := replaceBinary(binaryPath, newBinary, plan.CurrentVersion)
	if err != nil {
		return updateResult{}, err
	}

	result := updateResult{OldVersion: plan.CurrentVersion, NewVersion: plan.LatestVersion, BinaryPath: binaryPath, BackupPath: backup}
	if strings.TrimSpace(configPath) == "" {
		if configExplicit {
			return updateResult{}, errors.New("config path is empty")
		}
		return result, nil
	}
	added, skipped, err := upgradeConfigAppendMissing(configPath)
	if err != nil {
		return updateResult{}, err
	}
	result.ConfigPath = configPath
	result.ConfigAdded = added
	result.ConfigSkipped = skipped
	return result, nil
}

func getJSON(ctx context.Context, url string, out any) error {
	data, err := getBytes(ctx, url)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(data, out); err != nil {
		return fmt.Errorf("decode %s: %w", url, err)
	}
	return nil
}

func getBytes(ctx context.Context, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "go-review-update")
	resp, err := httpClientDo(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("GET %s: status %s", url, resp.Status)
	}
	return io.ReadAll(resp.Body)
}

func releaseArchiveName(version string, goos string, goarch string) string {
	ext := ".tar.gz"
	if goos == "windows" {
		ext = ".zip"
	}
	return fmt.Sprintf("go-review_%s_%s_%s%s", version, goos, goarch, ext)
}

func findReleaseAsset(assets []releaseAsset, name string) (releaseAsset, bool) {
	for _, asset := range assets {
		if asset.Name == name {
			return asset, true
		}
	}
	return releaseAsset{}, false
}

func verifyChecksum(assetName string, data []byte, checksums string) error {
	want := ""
	for _, line := range strings.Split(checksums, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		if filepath.Base(fields[len(fields)-1]) == assetName {
			want = strings.ToLower(fields[0])
			break
		}
	}
	if want == "" {
		return fmt.Errorf("checksums.txt missing %s", assetName)
	}
	sum := sha256.Sum256(data)
	got := hex.EncodeToString(sum[:])
	if got != want {
		return fmt.Errorf("checksum mismatch for %s: got %s want %s", assetName, got, want)
	}
	return nil
}

func extractGoReviewBinary(tmpDir string, archiveName string, data []byte) (string, error) {
	if strings.HasSuffix(archiveName, ".zip") {
		return extractBinaryFromZip(tmpDir, data)
	}
	return extractBinaryFromTarGz(tmpDir, data)
}

func extractBinaryFromTarGz(tmpDir string, data []byte) (string, error) {
	gz, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return "", err
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	for {
		header, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return "", err
		}
		if header.FileInfo().IsDir() || !isGoReviewBinaryName(filepath.Base(header.Name)) {
			continue
		}
		out := filepath.Join(tmpDir, filepath.Base(header.Name))
		if err := writeExecutable(out, tr); err != nil {
			return "", err
		}
		return out, nil
	}
	return "", errors.New("archive does not contain go-review binary")
}

func extractBinaryFromZip(tmpDir string, data []byte) (string, error) {
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return "", err
	}
	for _, file := range zr.File {
		if file.FileInfo().IsDir() || !isGoReviewBinaryName(filepath.Base(file.Name)) {
			continue
		}
		r, err := file.Open()
		if err != nil {
			return "", err
		}
		out := filepath.Join(tmpDir, filepath.Base(file.Name))
		err = writeExecutable(out, r)
		_ = r.Close()
		if err != nil {
			return "", err
		}
		return out, nil
	}
	return "", errors.New("archive does not contain go-review binary")
}

func isGoReviewBinaryName(name string) bool {
	return name == "go-review" || name == "go-review.exe"
}

func writeExecutable(path string, r io.Reader) error {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o755)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(f, r)
	closeErr := f.Close()
	if copyErr != nil {
		return copyErr
	}
	return closeErr
}

func replaceBinary(binaryPath string, newBinary string, currentVersion string) (string, error) {
	if strings.TrimSpace(binaryPath) == "" {
		return "", errors.New("binary path is empty")
	}
	absBinary, err := filepath.Abs(binaryPath)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(absBinary), 0o755); err != nil {
		return "", err
	}
	backup := fmt.Sprintf("%s.bak-%s", absBinary, sanitizeVersionForPath(currentVersion))
	if _, err := os.Stat(absBinary); err == nil {
		if err := copyFile(absBinary, backup, 0o755); err != nil {
			return "", fmt.Errorf("backup old binary: %w", err)
		}
	} else if err != nil && !os.IsNotExist(err) {
		return "", err
	}
	tmpNew := absBinary + ".new"
	if err := copyFile(newBinary, tmpNew, 0o755); err != nil {
		return "", fmt.Errorf("prepare new binary: %w", err)
	}
	if err := os.Rename(tmpNew, absBinary); err != nil {
		_ = os.Remove(tmpNew)
		return "", fmt.Errorf("replace binary: %w", err)
	}
	return backup, nil
}

func copyFile(src string, dst string, mode os.FileMode) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, mode)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(out, in)
	closeErr := out.Close()
	if copyErr != nil {
		return copyErr
	}
	return closeErr
}

func sanitizeVersionForPath(version string) string {
	version = strings.TrimSpace(version)
	if version == "" {
		return "unknown"
	}
	replacer := strings.NewReplacer("/", "-", "\\", "-", " ", "-")
	return replacer.Replace(version)
}

func resolveUpdateConfigPath(configPath string, workdir string) (string, bool, error) {
	if strings.TrimSpace(configPath) != "" {
		return configPath, true, nil
	}
	discovered, err := discoverConfig(workdir)
	if err != nil {
		return "", false, nil
	}
	return discovered, false, nil
}

func upgradeConfigAppendMissing(path string) ([]string, []string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, err
	}
	original := string(data)
	updated := original
	var added []string
	var skipped []string

	if strings.Contains(updated, "llm.claude:") {
		skipped = append(skipped, "tools.adapters.llm.claude")
	} else if next, ok := insertIntoToolsAdapters(updated, `    llm.claude: "claude"                 # optional second-pass runtime; disabled by default via steps[].enabled.`); ok {
		updated = next
		added = append(added, "tools.adapters.llm.claude")
	} else {
		skipped = append(skipped, "tools.adapters.llm.claude (未找到 tools.adapters 段)")
	}

	if configHasAdapterID(updated, "llm.claude") {
		skipped = append(skipped, "adapters[id=llm.claude]")
	} else if next, ok := insertIntoTopLevelList(updated, "adapters", `  - id: llm.claude
    type: llm.claude
    capabilities: [report]
    fix_safety: review`); ok {
		updated = next
		added = append(added, "adapters[id=llm.claude]")
	} else {
		skipped = append(skipped, "adapters[id=llm.claude] (未找到 adapters 段)")
	}

	if configHasStepID(updated, "llm-claude") {
		skipped = append(skipped, "steps[id=llm-claude]")
	} else if next, ok := insertIntoTopLevelList(updated, "steps", `  - id: llm-claude
    adapter: llm.claude
    enabled: false
    on_fail: continue`); ok {
		updated = next
		added = append(added, "steps[id=llm-claude]")
	} else {
		skipped = append(skipped, "steps[id=llm-claude] (未找到 steps 段)")
	}

	if profileHasStep(updated, "review", "llm-claude") {
		skipped = append(skipped, "profiles[review].steps += llm-claude")
	} else if configHasStepID(updated, "llm-claude") {
		if next, ok := appendStepToInlineProfile(updated, "review", "llm-claude"); ok {
			updated = next
			added = append(added, "profiles[review].steps += llm-claude")
		} else {
			skipped = append(skipped, "profiles[review].steps += llm-claude (未找到可安全修改的 review profile)")
		}
	}

	if updated == original {
		return added, skipped, nil
	}
	if _, err := config.Load(strings.NewReader(updated)); err != nil {
		return nil, nil, fmt.Errorf("merged config would be invalid, original file kept: %w", err)
	}
	if err := os.WriteFile(path, []byte(updated), 0o644); err != nil {
		return nil, nil, err
	}
	return added, skipped, nil
}

func insertIntoToolsAdapters(content string, line string) (string, bool) {
	lines := splitKeepLines(content)
	inTools := false
	for i, raw := range lines {
		trimmed := strings.TrimSpace(raw)
		if indentation(raw) == 0 {
			inTools = trimmed == "tools:"
			continue
		}
		if !inTools || indentation(raw) != 2 || trimmed != "adapters:" {
			continue
		}
		insertAt := i + 1
		for insertAt < len(lines) && indentation(lines[insertAt]) > 2 {
			insertAt++
		}
		return insertLines(lines, insertAt, []string{line + "\n"}), true
	}
	return content, false
}

func insertIntoTopLevelList(content string, key string, block string) (string, bool) {
	lines := splitKeepLines(content)
	for i, raw := range lines {
		if indentation(raw) != 0 || strings.TrimSpace(raw) != key+":" {
			continue
		}
		insertAt := i + 1
		for insertAt < len(lines) {
			trimmed := strings.TrimSpace(lines[insertAt])
			if trimmed != "" && !strings.HasPrefix(trimmed, "#") && indentation(lines[insertAt]) == 0 {
				break
			}
			insertAt++
		}
		return insertLines(lines, insertAt, ensureTrailingNewlines(block)), true
	}
	return content, false
}

func appendStepToInlineProfile(content string, profileName string, stepID string) (string, bool) {
	lines := splitKeepLines(content)
	inProfiles := false
	inTargetProfile := false
	for i, raw := range lines {
		trimmed := strings.TrimSpace(raw)
		if indentation(raw) == 0 {
			inProfiles = trimmed == "profiles:"
			inTargetProfile = false
			continue
		}
		if !inProfiles {
			continue
		}
		if indentation(raw) == 2 && strings.HasPrefix(trimmed, "- name:") {
			inTargetProfile = strings.TrimSpace(strings.TrimPrefix(trimmed, "- name:")) == profileName
			continue
		}
		if !inTargetProfile || indentation(raw) != 4 || !strings.HasPrefix(trimmed, "steps:") {
			continue
		}
		prefix, value, ok := strings.Cut(raw, "steps:")
		if !ok {
			return content, false
		}
		open := strings.Index(value, "[")
		close := strings.LastIndex(value, "]")
		if open < 0 || close < open {
			return content, false
		}
		inside := strings.TrimSpace(value[open+1 : close])
		parts := splitCommaList(inside)
		for _, part := range parts {
			if part == stepID {
				return content, true
			}
		}
		parts = append(parts, stepID)
		lines[i] = prefix + "steps: [" + strings.Join(parts, ", ") + "]" + lineEnding(raw)
		return strings.Join(lines, ""), true
	}
	return content, false
}

func configHasAdapterID(content string, id string) bool {
	return strings.Contains(content, "id: "+id) || strings.Contains(content, "id: \""+id+"\"")
}

func configHasStepID(content string, id string) bool {
	return strings.Contains(content, "id: "+id) || strings.Contains(content, "id: \""+id+"\"")
}

func profileHasStep(content string, profileName string, stepID string) bool {
	cfg, err := config.Load(strings.NewReader(content))
	if err != nil {
		return strings.Contains(content, stepID)
	}
	profile, err := cfg.Profile(profileName)
	if err != nil {
		return false
	}
	for _, step := range profile.Steps {
		if step == stepID {
			return true
		}
	}
	return false
}

func splitKeepLines(content string) []string {
	if content == "" {
		return nil
	}
	var lines []string
	start := 0
	for i, r := range content {
		if r == '\n' {
			lines = append(lines, content[start:i+1])
			start = i + 1
		}
	}
	if start < len(content) {
		lines = append(lines, content[start:])
	}
	return lines
}

func indentation(line string) int {
	count := 0
	for _, r := range line {
		switch r {
		case ' ':
			count++
		case '\t':
			count += 2
		default:
			return count
		}
	}
	return count
}

func insertLines(lines []string, index int, insert []string) string {
	out := make([]string, 0, len(lines)+len(insert))
	out = append(out, lines[:index]...)
	out = append(out, insert...)
	out = append(out, lines[index:]...)
	return strings.Join(out, "")
}

func ensureTrailingNewlines(block string) []string {
	var out []string
	for _, line := range strings.Split(block, "\n") {
		out = append(out, line+"\n")
	}
	return out
}

func lineEnding(line string) string {
	if strings.HasSuffix(line, "\n") {
		return "\n"
	}
	return ""
}

func splitCommaList(value string) []string {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	var out []string
	for _, part := range strings.Split(value, ",") {
		part = strings.TrimSpace(strings.Trim(part, "\"'"))
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}

func confirmUpdate(input io.Reader, output io.Writer) (bool, error) {
	fmt.Fprint(output, "是否升级？[y/N]: ")
	reader := bufio.NewReader(input)
	answer, err := reader.ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return false, err
	}
	answer = strings.ToLower(strings.TrimSpace(answer))
	return answer == "y" || answer == "yes", nil
}

func printUpdateAvailable(plan *updatePlan, w io.Writer) {
	fmt.Fprintf(w, "发现新版本：current=%s latest=%s\n", plan.CurrentVersion, plan.LatestVersion)
	if strings.TrimSpace(plan.ReleaseName) != "" {
		fmt.Fprintf(w, "Release: %s\n", plan.ReleaseName)
	}
	if summary := summarizeReleaseBody(plan.ReleaseBody); summary != "" {
		fmt.Fprintln(w, "\n更新概要：")
		fmt.Fprintln(w, summary)
	}
	fmt.Fprintln(w)
}

func summarizeReleaseBody(body string) string {
	body = strings.TrimSpace(body)
	if body == "" {
		return ""
	}
	lines := strings.Split(body, "\n")
	if len(lines) > 20 {
		lines = lines[:20]
		lines = append(lines, "...")
	}
	summary := strings.TrimSpace(strings.Join(lines, "\n"))
	const max = 2000
	if len(summary) > max {
		summary = strings.TrimSpace(summary[:max]) + "..."
	}
	return summary
}

func printUpdateResult(result updateResult, w io.Writer) {
	fmt.Fprintf(w, "UPDATED go-review %s -> %s\n", result.OldVersion, result.NewVersion)
	fmt.Fprintln(w, "\nBinary:")
	fmt.Fprintf(w, "  replaced: %s\n", result.BinaryPath)
	if result.BackupPath != "" {
		fmt.Fprintf(w, "  backup:   %s\n", result.BackupPath)
	}
	if result.ConfigPath == "" {
		fmt.Fprintln(w, "\nConfig: 未发现项目配置，跳过配置合并。")
		return
	}
	fmt.Fprintln(w, "\nConfig:")
	fmt.Fprintf(w, "  updated: %s\n", result.ConfigPath)
	printStringList(w, "Added", result.ConfigAdded, "+")
	printStringList(w, "Skipped", result.ConfigSkipped, "=")
}

func printStringList(w io.Writer, title string, values []string, marker string) {
	fmt.Fprintf(w, "\n%s:\n", title)
	if len(values) == 0 {
		fmt.Fprintf(w, "  %s none\n", marker)
		return
	}
	sorted := append([]string(nil), values...)
	sort.Strings(sorted)
	for _, value := range sorted {
		fmt.Fprintf(w, "  %s %s\n", marker, value)
	}
}

func compareVersions(current string, latest string) (int, error) {
	current = strings.TrimSpace(current)
	latest = strings.TrimSpace(latest)
	if latest == "" {
		return 0, errors.New("latest version is empty")
	}
	if current == "" || current == "dev" || current == "unknown" || strings.Contains(current, "devel") {
		return -1, nil
	}
	currentParts, err := parseVersionParts(current)
	if err != nil {
		return 0, fmt.Errorf("parse current version %q: %w", current, err)
	}
	latestParts, err := parseVersionParts(latest)
	if err != nil {
		return 0, fmt.Errorf("parse latest version %q: %w", latest, err)
	}
	for i := 0; i < 3; i++ {
		if currentParts[i] < latestParts[i] {
			return -1, nil
		}
		if currentParts[i] > latestParts[i] {
			return 1, nil
		}
	}
	return 0, nil
}

func parseVersionParts(version string) ([3]int, error) {
	var parts [3]int
	version = strings.TrimPrefix(strings.TrimSpace(version), "v")
	version, _, _ = strings.Cut(version, "-")
	items := strings.Split(version, ".")
	if len(items) == 0 || len(items) > 3 {
		return parts, fmt.Errorf("expected semantic version")
	}
	for i, item := range items {
		var value int
		if _, err := fmt.Sscanf(item, "%d", &value); err != nil {
			return parts, err
		}
		parts[i] = value
	}
	return parts, nil
}
