package main

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/v111nce/go-review/internal/config"
)

func TestRunVersionCommands(t *testing.T) {
	for _, args := range [][]string{{"version"}, {"--version"}, {"-v"}} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			stdout, stderr, code := captureRun(args)
			if code != 0 {
				t.Fatalf("run(%v) code=%d stderr=%q", args, code, stderr)
			}
			if !strings.Contains(stdout, "go-review version") || !strings.Contains(stdout, "commit") {
				t.Fatalf("version output missing fields: %q", stdout)
			}
			if stderr != "" {
				t.Fatalf("stderr=%q", stderr)
			}
		})
	}
}

func TestRunHelpMentionsDefaultCheck(t *testing.T) {
	stdout, stderr, code := captureRun([]string{"--help"})
	if code != 0 {
		t.Fatalf("help code=%d stderr=%q", code, stderr)
	}
	for _, want := range []string{"go-review [check]", "check    run configured adapters without applying edits (default; initializes missing config)", "version"} {
		if !strings.Contains(stdout, want) {
			t.Fatalf("help output missing %q:\n%s", want, stdout)
		}
	}
}

func TestRunDefaultsToCheckAndDiscoversRootConfig(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "go.mod"), "module example.com/default-check\n")
	writeFile(t, filepath.Join(dir, "main.go"), "package main\n\nfunc main() {}\n")
	writeFile(t, filepath.Join(dir, "go-review.yaml"), minimalConfig())
	reportDir := filepath.Join(dir, "reports")
	stdout, stderr, code := captureRun([]string{"--workdir", dir, "--report-dir", reportDir})
	if code != 0 {
		t.Fatalf("default check code=%d stderr=%q stdout=%q", code, stderr, stdout)
	}
	if !strings.Contains(stdout, "SUCCESS profile=fast") {
		t.Fatalf("default check output missing pass summary:\n%s", stdout)
	}
	if _, err := os.Stat(filepath.Join(reportDir, "latest.llm.md")); err != nil {
		t.Fatalf("expected LLM report: %v", err)
	}
}

func TestRunAutoInitializesMissingConfig(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "go.mod"), "module example.com/auto-init\n")
	writeFile(t, filepath.Join(dir, "main.go"), "package main\n\nfunc main() {}\n")
	stdout, stderr, code := captureRun([]string{"--workdir", dir})
	if code != 0 {
		t.Fatalf("auto init check code=%d stderr=%q stdout=%q", code, stderr, stdout)
	}
	for _, want := range []string{"initialized config=", "SUCCESS profile=fast"} {
		if !strings.Contains(stdout, want) {
			t.Fatalf("auto init output missing %q:\n%s", want, stdout)
		}
	}
	semanticDefault := filepath.Join(dir, ".go-review", "semantic", "default.yaml")
	for _, path := range []string{
		filepath.Join(dir, ".go-review", "go-review.yaml"),
		semanticDefault,
		filepath.Join(dir, ".go-review", "semantic", "custom.yaml"),
		filepath.Join(dir, ".go-review", "reports", "latest.md"),
		filepath.Join(dir, ".go-review", "reports", "latest.llm.md"),
		filepath.Join(dir, ".go-review", "artifacts", "latest", "format-check-stdout.txt"),
	} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("expected generated path %s: %v", path, err)
		}
	}
	semanticConfig, err := os.ReadFile(semanticDefault)
	if err != nil {
		t.Fatal(err)
	}
	semanticCustom, err := os.ReadFile(filepath.Join(dir, ".go-review", "semantic", "custom.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"rules:", "no-direct-os-getenv"} {
		if !strings.Contains(string(semanticConfig), want) {
			t.Fatalf("semantic default config missing %q:\n%s", want, semanticConfig)
		}
	}
	for _, want := range []string{"custom_rules:", "no-direct-call", "no-direct-fmt-println"} {
		if !strings.Contains(string(semanticCustom), want) {
			t.Fatalf("semantic custom config missing %q:\n%s", want, semanticCustom)
		}
	}
	if strings.Contains(string(semanticConfig), "exclude:") {
		t.Fatalf("semantic config should not own project exclude:\n%s", semanticConfig)
	}
	if strings.Contains(string(semanticCustom), "exclude:") {
		t.Fatalf("semantic custom config should not own project exclude:\n%s", semanticCustom)
	}
	projectConfig, err := os.ReadFile(filepath.Join(dir, ".go-review", "go-review.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	for _, line := range strings.Split(string(projectConfig), "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "#") {
			continue
		}
		if trimmed == "exclude:" || trimmed == "- vendor" || trimmed == "- testdata" {
			t.Fatalf("project config should not enable default exclude line %q:\n%s", trimmed, projectConfig)
		}
	}
	for _, want := range []string{
		"# exclude:",
		"# Optional: staticcheck. Install first:",
		"go install honnef.co/go/tools/cmd/staticcheck@latest",
		"go install golang.org/x/vuln/cmd/govulncheck@latest",
		"go install github.com/securego/gosec/v2/cmd/gosec@latest",
		"on_fail: continue",
	} {
		if !strings.Contains(string(projectConfig), want) {
			t.Fatalf("project config missing %q:\n%s", want, projectConfig)
		}
	}
}

func TestRunBareAutoInitializesFromProjectRoot(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "go.mod"), "module example.com/bare-auto-init\n")
	writeFile(t, filepath.Join(dir, "main.go"), "package main\n\nfunc main() {}\n")
	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(oldWD) })
	stdout, stderr, code := captureRun(nil)
	if code != 0 {
		t.Fatalf("bare auto init code=%d stderr=%q stdout=%q", code, stderr, stdout)
	}
	if strings.Contains(stdout, ".go-review/.go-review") {
		t.Fatalf("generated config should resolve artifacts from project root, stdout=%s", stdout)
	}
	if _, err := os.Stat(filepath.Join(dir, ".go-review", "reports", "latest.llm.md")); err != nil {
		t.Fatalf("expected report under project .go-review: %v", err)
	}
}

func TestDefaultConfigKeepsReviewAreasIndependent(t *testing.T) {
	cfg, err := config.Load(strings.NewReader(defaultConfig()))
	if err != nil {
		t.Fatalf("Load(defaultConfig): %v", err)
	}
	if len(cfg.Exclude) != 0 {
		t.Fatalf("default config should not enable project excludes: %#v", cfg.Exclude)
	}
	for _, step := range cfg.Steps {
		if step.OnFail != config.OnFailContinue {
			t.Fatalf("step %s on_fail = %q, want continue", step.ID, step.OnFail)
		}
	}
}

func TestRunInitCreatesConfigOnly(t *testing.T) {
	dir := t.TempDir()
	stdout, stderr, code := captureRun([]string{"init", "--workdir", dir})
	if code != 0 {
		t.Fatalf("init code=%d stderr=%q stdout=%q", code, stderr, stdout)
	}
	if !strings.Contains(stdout, "initialized config=") {
		t.Fatalf("init output missing initialized config: %s", stdout)
	}
	if _, err := os.Stat(filepath.Join(dir, ".go-review", "go-review.yaml")); err != nil {
		t.Fatalf("expected config: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".go-review", "semantic", "default.yaml")); err != nil {
		t.Fatalf("expected semantic default config: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".go-review", "reports")); !os.IsNotExist(err) {
		t.Fatalf("init should not create reports before running checks, err=%v", err)
	}
}

func TestRunInitKeepsExistingConfig(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, ".go-review"), 0o755); err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(dir, ".go-review", "go-review.yaml")
	writeFile(t, configPath, minimalConfig())
	stdout, stderr, code := captureRun([]string{"init", "--workdir", dir})
	if code != 0 {
		t.Fatalf("init existing code=%d stderr=%q stdout=%q", code, stderr, stdout)
	}
	if !strings.Contains(stdout, configPath) {
		t.Fatalf("init existing output = %s, want %s", stdout, configPath)
	}
}

func TestDiscoverConfigPrefersDotGoReview(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, ".go-review"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(dir, "go-review.yaml"), minimalConfig())
	writeFile(t, filepath.Join(dir, ".go-review", "go-review.yaml"), minimalConfig())
	got, err := discoverConfig(dir)
	if err != nil {
		t.Fatalf("discoverConfig: %v", err)
	}
	want := filepath.Join(dir, ".go-review", "go-review.yaml")
	if got != want {
		t.Fatalf("discoverConfig = %q, want %q", got, want)
	}
}

func TestDiscoverConfigMissing(t *testing.T) {
	_, err := discoverConfig(t.TempDir())
	if err == nil || !strings.Contains(err.Error(), "no config found") {
		t.Fatalf("discoverConfig missing err = %v", err)
	}
}

func TestDefaultReportDir(t *testing.T) {
	cases := map[string]string{
		"go-review.yaml":            ".go-review/reports",
		"configs/go-review.yaml":    "configs/.go-review/reports",
		".go-review/go-review.yaml": ".go-review/reports",
		"":                          "",
	}
	for input, want := range cases {
		if got := defaultReportDir(input); got != want {
			t.Fatalf("defaultReportDir(%q) = %q, want %q", input, got, want)
		}
	}
}

func captureRun(args []string) (stdout string, stderr string, code int) {
	oldStdout := os.Stdout
	oldStderr := os.Stderr
	outR, outW, _ := os.Pipe()
	errR, errW, _ := os.Pipe()
	os.Stdout = outW
	os.Stderr = errW
	code = run(args)
	_ = outW.Close()
	_ = errW.Close()
	os.Stdout = oldStdout
	os.Stderr = oldStderr
	var outBuf, errBuf bytes.Buffer
	_, _ = io.Copy(&outBuf, outR)
	_, _ = io.Copy(&errBuf, errR)
	return outBuf.String(), errBuf.String(), code
}

func TestBuildMetadataPrefersLDFlags(t *testing.T) {
	oldVersion, oldCommit, oldDate := version, commit, date
	t.Cleanup(func() {
		version, commit, date = oldVersion, oldCommit, oldDate
	})

	version, commit, date = "v-test", "commit-test", "date-test"
	gotVersion, gotCommit, gotDate := buildMetadata()
	if gotVersion != "v-test" || gotCommit != "commit-test" || gotDate != "date-test" {
		t.Fatalf("buildMetadata() = %q, %q, %q", gotVersion, gotCommit, gotDate)
	}
}

func writeFile(t *testing.T, path string, data string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
}

func minimalConfig() string {
	return `schema_version: "1.0"
defaults:
  workdir: .
adapters:
  - id: go.lint
    type: go.lint
    fix_safety: safe
steps:
  - id: format-check
    adapter: go.lint
    allow_fix: true
profiles:
  - name: fast
    steps: [format-check]
`
}
