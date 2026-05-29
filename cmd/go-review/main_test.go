package main

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/v111nce/go-review/internal/config"
	"github.com/v111nce/go-review/internal/rulecatalog"
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
	if _, err := os.Stat(filepath.Join(reportDir, "latest.process.md")); err != nil {
		t.Fatalf("expected process report: %v", err)
	}
}

// TestRunAutoInitializesMissingConfig 验证首次运行时会自动生成默认配置、semantic 配置、
// 本地规则 catalog 和报告目录，并且默认配置中只把用户自有 exclude 作为注释提示。
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
	golangciConfig := filepath.Join(dir, ".go-review", "golangci.yml")
	rulesCatalog := filepath.Join(dir, ".go-review", "rules.json")
	llmRules := filepath.Join(dir, ".go-review", "llm-rules.json")
	for _, path := range []string{
		filepath.Join(dir, ".go-review", "go-review.yaml"),
		golangciConfig,
		semanticDefault,
		filepath.Join(dir, ".go-review", "semantic", "custom.yaml"),
		rulesCatalog,
		llmRules,
		filepath.Join(dir, ".go-review", "reports", "latest.md"),
		filepath.Join(dir, ".go-review", "reports", "latest.llm.md"),
		filepath.Join(dir, ".go-review", "reports", "latest.process.md"),
		filepath.Join(dir, ".go-review", "artifacts", "latest", "format-check-stdout.txt"),
	} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("expected generated path %s: %v", path, err)
		}
	}
	rulesData, err := os.ReadFile(rulesCatalog)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"go-review.rules.v1", "go.official.gofmt", "team.semantic.max-params"} {
		if !strings.Contains(string(rulesData), want) {
			t.Fatalf("rules catalog missing %q:\n%s", want, rulesData)
		}
	}

	llmRulesData, err := os.ReadFile(llmRules)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"go-review.rules.v1", "llm-review", "go.official.goroutine-lifetimes", "uber.guideline.no-fire-forget"} {
		if !strings.Contains(string(llmRulesData), want) {
			t.Fatalf("llm rules missing %q:\n%s", want, llmRulesData)
		}
	}

	llmReport, err := os.ReadFile(filepath.Join(dir, ".go-review", "reports", "latest.llm.md"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"## LLM 审阅规则", ".go-review/llm-rules.json", "LLM 规则数量", "rule_id"} {
		if !strings.Contains(string(llmReport), want) {
			t.Fatalf("llm report missing %q:\n%s", want, llmReport)
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
	for _, want := range []string{"rules:", "import-blank", "custom-contexts", "no-tfatal-goroutine", "channel-size", "enum-start-one", "exit-in-main", "no-direct-os-getenv"} {
		if !strings.Contains(string(semanticConfig), want) {
			t.Fatalf("semantic default config missing %q:\n%s", want, semanticConfig)
		}
	}
	for _, want := range []string{"rules:", "no-direct-call", "no-direct-fmt-println"} {
		if !strings.Contains(string(semanticCustom), want) {
			t.Fatalf("semantic custom config missing %q:\n%s", want, semanticCustom)
		}
	}
	golangciData, err := os.ReadFile(golangciConfig)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"version: \"2\"", "SA5008", "staticcheck"} {
		if !strings.Contains(string(golangciData), want) {
			t.Fatalf("golangci config missing %q:\n%s", want, golangciData)
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
		"llm.review: \"codex\"",
		"type: llm.review",
		"enabled: false",
		"steps: [format-check, lint, test, semantic, llm-review, llm-claude]",
		"on_fail: continue",
		"--config=.go-review/golangci.yml",
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

// TestDefaultConfigKeepsReviewAreasIndependent 锁定默认 pipeline 的失败隔离策略。
// format/lint/test/semantic 任一部分失败，都不能阻止其它部分继续运行。
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
	if _, err := os.Stat(filepath.Join(dir, ".go-review", "rules.json")); err != nil {
		t.Fatalf("expected rules catalog: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".go-review", "llm-rules.json")); err != nil {
		t.Fatalf("expected llm rules catalog: %v", err)
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
	for _, path := range []string{
		filepath.Join(dir, ".go-review", "rules.json"),
		filepath.Join(dir, ".go-review", "llm-rules.json"),
		filepath.Join(dir, ".go-review", "semantic", "default.yaml"),
		filepath.Join(dir, ".go-review", "semantic", "custom.yaml"),
	} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("expected companion file %s: %v", path, err)
		}
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

// TestDefaultConfigCoversLowNoiseGolangciRules 锁定默认低噪声 golangci 规则承接。
//
// catalog 中有些规则是 strict/team 风格规则，例如 paralleltest、varnamelen、revive
// 注释句式等；它们可以保持 implemented，但不应强制进入通用默认配置。这里仅要求
// default_profile 属于 default/ci 的直接工具规则被默认 go.lint 参数覆盖。
func TestDefaultConfigCoversLowNoiseGolangciRules(t *testing.T) {
	cfg, err := config.Load(strings.NewReader(defaultConfig()))
	if err != nil {
		t.Fatalf("Load(defaultConfig): %v", err)
	}
	catalog, err := rulecatalog.LoadFile("../../rules/go-rules.json")
	if err != nil {
		t.Fatalf("LoadFile: %v", err)
	}
	enabled := defaultGolangciToolRules(t, cfg)
	for _, rule := range catalog.Rules {
		if rule.Handling != "tool-golangci" {
			continue
		}
		if !isDefaultGateProfile(rule.DefaultProfile) {
			continue
		}
		if !rule.Implemented {
			t.Fatalf("default golangci rule %s should be implemented", rule.ID)
		}
		if rule.Adapter != "go.lint" {
			t.Fatalf("default golangci rule %s adapter=%q, want go.lint", rule.ID, rule.Adapter)
		}
		if len(rule.ToolRules) == 0 {
			t.Fatalf("default golangci rule %s missing tool_rules", rule.ID)
		}
		covered := false
		for _, toolRule := range rule.ToolRules {
			if defaultToolRuleCovered(enabled, toolRule) {
				covered = true
				break
			}
		}
		if !covered {
			t.Fatalf("default golangci rule %s tool_rules=%v not covered by default go.lint args; enabled=%v", rule.ID, rule.ToolRules, enabled)
		}
	}
}

// TestCatalogToolRulesHaveKnownExecutionOwners 锁定 A 类所有 tool_rules 的承接边界。
// direct 规则必须被默认配置启用；config 规则允许先进入已知配置型工具清单，但不能出现未知工具名。
func TestCatalogToolRulesHaveKnownExecutionOwners(t *testing.T) {
	cfg, err := config.Load(strings.NewReader(defaultConfig()))
	if err != nil {
		t.Fatalf("Load(defaultConfig): %v", err)
	}
	catalog, err := rulecatalog.LoadFile("../../rules/go-rules.json")
	if err != nil {
		t.Fatalf("LoadFile: %v", err)
	}
	enabled := defaultGolangciToolRules(t, cfg)
	knownConfig := map[string]bool{
		"atomic linters":               true,
		"denylist":                     true,
		"dep policy":                   true,
		"depguard":                     true,
		"gci":                          true,
		"forbidigo":                    true,
		"go.semantic":                  true,
		"govet composites":             true,
		"govet printf":                 true,
		"govet printf analyzer config": true,
		"godoclint":                    true,
		"godot":                        true,
		"nakedret":                     true,
		"nonamedreturns":               true,
		"paralleltest":                 true,
		"varnamelen":                   true,
		"gochecknoglobals":             true,
		"gochecknoinits":               true,
		"gocritic":                     true,
		"perfsprint":                   true,
		"thelper":                      true,
		"testifylint":                  true,
		"importas":                     true,
		"makezero":                     true,
		"prealloc":                     true,
		"package denylist":             true,
		"perf linters":                 true,
		"revive":                       true,
		"revive error-strings":         true,
		"revive naming":                true,
		"semantic":                     true,
	}
	for _, rule := range catalog.Rules {
		switch rule.Handling {
		case "tool-golangci":
			for _, toolRule := range rule.ToolRules {
				if !defaultToolRuleCovered(enabled, toolRule) && !knownConfig[toolRule] {
					t.Fatalf("direct golangci rule %s has unknown/unenabled tool rule %q", rule.ID, toolRule)
				}
			}
		case "tool-golangci-config":
			if rule.Adapter != "go.lint" {
				t.Fatalf("config golangci rule %s adapter=%q, want go.lint", rule.ID, rule.Adapter)
			}
			for _, toolRule := range rule.ToolRules {
				if !defaultToolRuleCovered(enabled, toolRule) && !knownConfig[toolRule] {
					t.Fatalf("config golangci rule %s has unknown tool rule %q", rule.ID, toolRule)
				}
			}
		}
	}
}

func isDefaultGateProfile(profile string) bool {
	switch profile {
	case "", "default", "ci":
		return true
	default:
		return false
	}
}

func defaultToolRuleCovered(enabled map[string]bool, toolRule string) bool {
	toolRule = strings.TrimSpace(toolRule)
	if enabled[toolRule] {
		return true
	}
	if head, _, ok := strings.Cut(toolRule, " "); ok && enabled[head] {
		return true
	}
	switch toolRule {
	case "gci":
		return enabled["goimports"]
	case "gofumpt":
		return enabled["gofmt"]
	default:
		return false
	}
}

func defaultGolangciToolRules(t *testing.T, cfg *config.Config) map[string]bool {
	t.Helper()
	enabled := map[string]bool{"go.lint": true}
	for _, adapter := range cfg.Adapters {
		if adapter.Type != "go.lint" {
			continue
		}
		for i := 0; i < len(adapter.Args); i++ {
			arg := adapter.Args[i]
			switch {
			case arg == "--enable" || arg == "-E":
				if i+1 >= len(adapter.Args) {
					t.Fatalf("adapter %s has %s without value", adapter.ID, arg)
				}
				markToolRules(enabled, adapter.Args[i+1])
				i++
			case arg == "--enable-only":
				if i+1 >= len(adapter.Args) {
					t.Fatalf("adapter %s has --enable-only without value", adapter.ID)
				}
				markToolRules(enabled, adapter.Args[i+1])
				i++
			case strings.HasPrefix(arg, "--enable="):
				markToolRules(enabled, strings.TrimPrefix(arg, "--enable="))
			case strings.HasPrefix(arg, "-E="):
				markToolRules(enabled, strings.TrimPrefix(arg, "-E="))
			case strings.HasPrefix(arg, "--enable-only="):
				markToolRules(enabled, strings.TrimPrefix(arg, "--enable-only="))
			}
		}
	}
	return enabled
}

func markToolRules(enabled map[string]bool, value string) {
	for _, part := range strings.Split(value, ",") {
		part = strings.TrimSpace(part)
		if part != "" {
			enabled[part] = true
		}
	}
}

func TestRulesCRUDAndRenderDoc(t *testing.T) {
	dir := t.TempDir()
	catalog := filepath.Join(dir, "rules.json")
	rule := `{
  "id": "team.semantic.max-params",
  "title": "函数参数上限",
  "description": "函数/方法入参个数不得超过配置阈值。",
  "source": {"name": "Team semantic rules", "section": "max-params"},
  "handling": "tool-semantic",
  "adapter": "go.semantic",
  "tool_rules": ["max-params"],
  "default_profile": "strict",
  "severity": "medium",
  "autofix": {"supported": false, "safety": "none"},
  "status": "active",
  "implemented": true,
  "notes": "确定性 AST 子规则。"
}`
	stdout, stderr, code := captureRun([]string{"rules", "add", "--catalog", catalog, rule})
	if code != 0 {
		t.Fatalf("rules add code=%d stderr=%q stdout=%q", code, stderr, stdout)
	}
	stdout, stderr, code = captureRun([]string{"rules", "list", "--catalog", catalog})
	if code != 0 || !strings.Contains(stdout, "team.semantic.max-params") || !strings.Contains(stdout, "implemented=true") || !strings.Contains(stdout, "函数/方法入参") {
		t.Fatalf("rules list code=%d stderr=%q stdout=%q", code, stderr, stdout)
	}
	stdout, stderr, code = captureRun([]string{"rules", "get", "--catalog", catalog, "team.semantic.max-params"})
	if code != 0 || !strings.Contains(stdout, "\"handling\": \"tool-semantic\"") {
		t.Fatalf("rules get code=%d stderr=%q stdout=%q", code, stderr, stdout)
	}
	docPath := filepath.Join(dir, "catalog.md")
	stdout, stderr, code = captureRun([]string{"rules", "render-doc", "--catalog", catalog, "--out", docPath})
	if code != 0 {
		t.Fatalf("rules render-doc code=%d stderr=%q stdout=%q", code, stderr, stdout)
	}
	doc, err := os.ReadFile(docPath)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"由 JSON catalog 生成", "team.semantic.max-params", "函数/方法入参个数不得超过配置阈值", "yes"} {
		if !strings.Contains(string(doc), want) {
			t.Fatalf("rendered doc missing %q:\n%s", want, doc)
		}
	}
	stdout, stderr, code = captureRun([]string{"rules", "delete", "--catalog", catalog, "team.semantic.max-params"})
	if code != 0 {
		t.Fatalf("rules delete code=%d stderr=%q stdout=%q", code, stderr, stdout)
	}
	stdout, stderr, code = captureRun([]string{"rules", "validate", "--catalog", catalog})
	if code != 0 || !strings.Contains(stdout, "rules=0") {
		t.Fatalf("rules validate code=%d stderr=%q stdout=%q", code, stderr, stdout)
	}
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
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
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
