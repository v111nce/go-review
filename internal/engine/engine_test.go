package engine

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/v111nce/go-review/internal/config"
)

// 验证 cmd adapter 正常执行时返回 pass，执行失败时返回 fail，并且产出 artifact 文件路径。
func TestCommandAdapterSuccessFailureAndArtifacts(t *testing.T) {
	dir := t.TempDir()
	cfg := writeConfig(t, dir, `schema_version: "1.0"
defaults:
  timeout: 2s
  workdir: .
adapters:
  - id: ok
    type: cmd
    command: sh
    args: [-c, "echo ok"]
    capabilities: [check]
  - id: fail
    type: cmd
    command: sh
    args: [-c, "echo bad >&2; exit 3"]
    capabilities: [check]
steps:
  - id: ok-step
    adapter: ok
    on_fail: continue
  - id: fail-step
    adapter: fail
    on_fail: continue
profiles:
  - name: fast
    steps: [ok-step, fail-step]
artifacts:
  dir: artifacts
`)
	summary, err := NewRunner().Run(context.Background(), Options{Command: CommandCheck, Config: cfg, Profile: "fast"})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if len(summary.Results) != 2 {
		t.Fatalf("results = %d", len(summary.Results))
	}
	byStep := map[string]Result{}
	for _, result := range summary.Results {
		byStep[result.StepID] = result
	}
	if byStep["ok-step"].GateStatus != config.GatePass {
		t.Fatalf("ok result = %#v", byStep["ok-step"])
	}
	if byStep["fail-step"].GateStatus != config.GateFail {
		t.Fatalf("fail result = %#v", byStep["fail-step"])
	}
	if byStep["ok-step"].Artifacts[0].Path == "" {
		t.Fatal("expected stdout artifact path")
	}
}

// 验证 cmd adapter 超时后返回 fail，不会一直阻塞。
func TestCommandAdapterTimeout(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell fixture uses sh")
	}
	dir := t.TempDir()
	cfg := writeConfig(t, dir, `schema_version: "1.0"
defaults:
  workdir: .
adapters:
  - id: slow
    type: cmd
    command: sh
    args: [-c, "sleep 1"]
    timeout: 10ms
steps:
  - id: slow-step
    adapter: slow
profiles:
  - name: fast
    steps: [slow-step]
`)
	summary, err := NewRunner().Run(context.Background(), Options{Command: CommandCheck, Config: cfg, Profile: "fast"})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if got := summary.Results[0].GateStatus; got != config.GateFail {
		t.Fatalf("timeout gate = %s", got)
	}
}

// 验证 cmd adapter 能正确注入环境变量，并在指定 workdir 下执行。
func TestCommandAdapterEnvAndWorkdir(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	cfg := writeConfig(t, dir, `schema_version: "1.0"
defaults:
  workdir: .
adapters:
  - id: env
    type: cmd
    command: sh
    args: [-c, "printf '%s:%s' "$REVIEW_ENV" "$(basename "$PWD")""]
    workdir: sub
    env:
      REVIEW_ENV: ok
steps:
  - id: env-step
    adapter: env
profiles:
  - name: fast
    steps: [env-step]
`)
	summary, err := NewRunner().Run(context.Background(), Options{Command: CommandCheck, Config: cfg, Profile: "fast"})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if got := summary.Results[0].Message; got != "ok:sub" {
		t.Fatalf("message = %q", got)
	}
}

// 验证 go.lint adapter 在 check 模式下发现未格式化文件时返回 fail，并标记 fix 可用。
func TestGoLintCheckDetectsUnformattedFile(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "bad.go"), []byte("package main\nfunc main(){\n}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg := writeConfig(t, dir, fmt.Sprintf(`schema_version: "1.0"
defaults:
  workdir: .
adapters:
  - id: format
    type: go.lint
    command: %s
    fix_safety: safe
steps:
  - id: format-step
    adapter: format
profiles:
  - name: fast
    steps: [format-step]
`, fakeGolangciLint(t)))
	summary, err := NewRunner().Run(context.Background(), Options{Command: CommandCheck, Config: cfg, Profile: "fast"})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if got := summary.Results[0].GateStatus; got != config.GateFail {
		t.Fatalf("format gate = %s", got)
	}
	if summary.Results[0].RuleID != "go.official.gofmt" {
		t.Fatalf("format rule id = %q", summary.Results[0].RuleID)
	}
	if !summary.Results[0].FixAvailable {
		t.Fatal("expected fix availability")
	}
}

// 验证 go.lint adapter 在 goimports/gci formatter 配置下输出 catalog import rule_id。
func TestGoLintCheckMapsImportFormatterRuleID(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "bad.go"), []byte("package main\nfunc main(){\n}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg := writeConfig(t, dir, fmt.Sprintf(`schema_version: "1.0"
defaults:
  workdir: .
adapters:
  - id: imports
    type: go.lint
    command: %s
    args: [fmt, --no-config, --enable, goimports]
    fix_safety: safe
steps:
  - id: import-step
    adapter: imports
profiles:
  - name: fast
    steps: [import-step]
`, fakeGolangciLint(t)))
	summary, err := NewRunner().Run(context.Background(), Options{Command: CommandCheck, Config: cfg, Profile: "fast"})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	got := summary.Results[0]
	if got.GateStatus != config.GateFail || got.RuleID != "go.official.imports" || !got.FixAvailable {
		t.Fatalf("import formatter result = %#v", got)
	}
}

// 验证 go.lint adapter 能把 errcheck 输出映射成 catalog error-handling rule_id。
func TestGoLintRunMapsErrcheckRuleID(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\nimport \"fmt\"\nfunc main() {\n\tfmt.Errorf(\"ignored\")\n}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg := writeConfig(t, dir, fmt.Sprintf(`schema_version: "1.0"
defaults:
  workdir: .
adapters:
  - id: lint
    type: go.lint
    command: %s
    args: [run, --no-config, --enable, errcheck]
steps:
  - id: lint-step
    adapter: lint
profiles:
  - name: fast
    steps: [lint-step]
`, fakeGolangciLint(t)))
	summary, err := NewRunner().Run(context.Background(), Options{Command: CommandCheck, Config: cfg, Profile: "fast"})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	got := summary.Results[0]
	if got.GateStatus != config.GateFail || got.RuleID != "go.official.handle-errors" || got.File != "main.go" || got.Line != 4 || got.Column != 12 {
		t.Fatalf("errcheck result = %#v", got)
	}
	if !strings.Contains(got.Message, "Error return value") {
		t.Fatalf("errcheck message = %q", got.Message)
	}
}

// 验证 go.lint adapter 能从 golangci-lint 输出中解析更多 linter，并映射到 catalog rule_id。
func TestGoLintRunMapsCatalogRuleIDsFromLinters(t *testing.T) {
	tests := []struct {
		name    string
		source  string
		args    string
		wantID  string
		wantMsg string
	}{
		{
			name:    "revive",
			source:  "package main\n\nfunc BadName() {}\n",
			args:    "[run, --no-config, --enable-only, revive]",
			wantID:  "go.official.identifier-style",
			wantMsg: "exported function",
		},
		{
			name:    "gosec",
			source:  "package main\nimport \"math/rand\"\nfunc main() {\n\t_ = rand.Int()\n}\n",
			args:    "[run, --no-config, --enable-only=gosec]",
			wantID:  "go.official.crypto-rand",
			wantMsg: "weak random",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte(tt.source), 0o644); err != nil {
				t.Fatal(err)
			}
			cfg := writeConfig(t, dir, fmt.Sprintf(`schema_version: "1.0"
defaults:
  workdir: .
adapters:
  - id: lint
    type: go.lint
    command: %s
    args: %s
steps:
  - id: lint-step
    adapter: lint
profiles:
  - name: fast
    steps: [lint-step]
`, fakeGolangciLint(t), tt.args))
			summary, err := NewRunner().Run(context.Background(), Options{Command: CommandCheck, Config: cfg, Profile: "fast"})
			if err != nil {
				t.Fatalf("Run() error = %v", err)
			}
			got := summary.Results[0]
			if got.GateStatus != config.GateFail || got.RuleID != tt.wantID || got.File != "main.go" {
				t.Fatalf("lint mapped result = %#v", got)
			}
			if !strings.Contains(got.Message, tt.wantMsg) {
				t.Fatalf("lint message = %q, want %q", got.Message, tt.wantMsg)
			}
		})
	}
}

// 验证 fix_safety=review 的 adapter 在 fix 模式下不会自动应用修改，文件保持原样。
func TestGoLintFixRequiresSafeAllowedStep(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "bad.go")
	if err := os.WriteFile(file, []byte("package main\nfunc main(){\n}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg := writeConfig(t, dir, fmt.Sprintf(`schema_version: "1.0"
defaults:
  workdir: .
adapters:
  - id: format
    type: go.lint
    command: %s
    fix_safety: review
steps:
  - id: format-step
    adapter: format
    allow_fix: true
profiles:
  - name: fast
    steps: [format-step]
`, fakeGolangciLint(t)))
	summary, err := NewRunner().Run(context.Background(), Options{Command: CommandFix, Config: cfg, Profile: "fast"})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if summary.Results[0].GateStatus != config.GateFail || summary.Results[0].FixApplied {
		t.Fatalf("non-safe fix should not apply: %#v", summary.Results[0])
	}
	data, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "main(){") {
		t.Fatalf("non-safe fix changed file: %s", data)
	}
}

// 验证 fix 应用后若后续校验（如测试）失败，会自动回滚文件到修改前的状态。
func TestFixRollsBackWhenDependentValidationFails(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "bad.go"), []byte("package main\nfunc main(){println(Message())}\nfunc Message() string { return \"wrong\" }\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "bad_test.go"), []byte("package main\nimport \"testing\"\nfunc TestMessage(t *testing.T) { if Message() != \"right\" { t.Fatal(\"validation failed\") } }\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg := writeConfig(t, dir, fmt.Sprintf(`schema_version: "1.0"
defaults:
  workdir: .
adapters:
  - id: format
    type: go.lint
    command: %s
    fix_safety: safe
  - id: test
    type: cmd
    command: go
    args: [test, ./...]
steps:
  - id: format-step
    adapter: format
    allow_fix: true
  - id: test-step
    adapter: test
    depends_on: [format-step]
profiles:
  - name: fast
    steps: [format-step, test-step]
`, fakeGolangciLint(t)))
	summary, err := NewRunner().Run(context.Background(), Options{Command: CommandFix, Config: cfg, Profile: "fast"})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if len(summary.Results) < 3 || summary.Results[len(summary.Results)-1].AdapterID != "fix.transaction" {
		t.Fatalf("expected rollback evidence, results=%#v", summary.Results)
	}
	data, err := os.ReadFile(filepath.Join(dir, "bad.go"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "main(){println") {
		t.Fatalf("rollback did not restore original formatting: %s", data)
	}
}

// 验证内置规则 no-direct-os-getenv 能检测到直接调用 os.Getenv，并报告正确的文件、行号和修复建议。
func TestSemanticAdapterDetectsDirectOSGetenv(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\nimport \"os\"\nfunc Message() string { return os.Getenv(\"MESSAGE\") }\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg := writeConfig(t, dir, `schema_version: "1.0"
defaults:
  workdir: .
adapters:
  - id: semantic.no-env
    type: go.semantic
    fix_safety: review
steps:
  - id: semantic-step
    adapter: semantic.no-env
profiles:
  - name: fast
    steps: [semantic-step]
`)
	if err := os.MkdirAll(filepath.Join(dir, "semantic"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "semantic", "default.yaml"), []byte("rules:\n  - no-direct-os-getenv\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	summary, err := NewRunner().Run(context.Background(), Options{Command: CommandCheck, Config: cfg, Profile: "fast"})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	got := summary.Results[0]
	if got.GateStatus != config.GateFail || got.RuleID != noDirectEnvRuleID || got.File != "main.go" || got.Line == 0 || got.Suggestion == "" {
		t.Fatalf("semantic result = %#v", got)
	}
}

// 验证 on_fail: stop 时第一个 step 失败后不再继续执行后续 step。
func TestRunStopsOnFailure(t *testing.T) {
	dir := t.TempDir()
	cfg := writeConfig(t, dir, `schema_version: "1.0"
adapters:
  - id: fail
    type: cmd
    command: sh
    args: [-c, "exit 1"]
  - id: ok
    type: cmd
    command: sh
    args: [-c, "echo ok"]
steps:
  - id: fail-step
    adapter: fail
    on_fail: stop
  - id: ok-step
    adapter: ok
profiles:
  - name: fast
    steps: [fail-step, ok-step]
`)
	summary, err := NewRunner().Run(context.Background(), Options{Command: CommandCheck, Config: cfg, Profile: "fast"})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if len(summary.Results) != 1 {
		t.Fatalf("results = %d", len(summary.Results))
	}
}

// 验证 depends_on 依赖关系能正确排序执行顺序，即使 profile 里 step 顺序写反了。
func TestRunOrdersProfileByDependencies(t *testing.T) {
	dir := t.TempDir()
	cfg := writeConfig(t, dir, `schema_version: "1.0"
adapters:
  - id: first
    type: cmd
    command: sh
    args: [-c, "printf first"]
  - id: second
    type: cmd
    command: sh
    args: [-c, "printf second"]
steps:
  - id: second-step
    adapter: second
    depends_on: [first-step]
  - id: first-step
    adapter: first
profiles:
  - name: fast
    steps: [second-step, first-step]
`)
	summary, err := NewRunner().Run(context.Background(), Options{Command: CommandCheck, Config: cfg, Profile: "fast"})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if got := []string{summary.Results[0].StepID, summary.Results[1].StepID}; got[0] != "first-step" || got[1] != "second-step" {
		t.Fatalf("execution order = %v", got)
	}
}

func writeConfig(t *testing.T, dir, body string) string {
	t.Helper()
	path := filepath.Join(dir, "go-review.yml")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// 验证 exit code：warn 不影响退出码，fail 返回非零退出码。
func TestSummaryExitCode(t *testing.T) {
	s := RunSummary{Results: []Result{{GateStatus: config.GateWarn}}}
	if s.ExitCode() != 0 {
		t.Fatal("warn should not fail exit code")
	}
	s.Results = append(s.Results, Result{GateStatus: config.GateFail})
	if s.ExitCode() != 1 {
		t.Fatal("fail should return non-zero exit code")
	}
}

// 验证 Registry 解析不存在的 adapter type 时返回错误。
func TestRegistryRejectsUnknownAdapter(t *testing.T) {
	_, err := NewRegistry().Resolve(config.Adapter{ID: "x", Type: "missing"})
	if err == nil {
		t.Fatal("expected unknown adapter error")
	}
}

// 验证 step 级别的 timeout 优先级高于 adapter 级别的 timeout。
func TestTimeoutFromStepOverridesAdapter(t *testing.T) {
	r := CommandAdapter{cfg: config.Adapter{ID: "slow", Command: "sh", Args: []string{"-c", "sleep 1"}, Timeout: time.Second}}
	result, _ := r.Run(context.Background(), StepContext{Step: config.Step{ID: "s", Timeout: time.Millisecond}, Adapter: config.Adapter{ID: "slow"}, Config: &config.Config{}, ProjectRoot: "."})
	if result.GateStatus != config.GateFail {
		t.Fatalf("gate = %s", result.GateStatus)
	}
}

// 验证显式传入 --workdir 时路径相对于调用目录解析，而非配置文件目录。
func TestExplicitWorkdirIsRelativeToInvocationDirectory(t *testing.T) {
	dir := t.TempDir()
	project := filepath.Join(dir, "project")
	if err := os.Mkdir(project, 0o755); err != nil {
		t.Fatal(err)
	}
	cfg := writeConfig(t, dir, `schema_version: "1.0"
defaults:
  workdir: .
adapters:
  - id: pwd
    type: cmd
    command: sh
    args: [-c, "basename "$PWD""]
steps:
  - id: pwd-step
    adapter: pwd
profiles:
  - name: fast
    steps: [pwd-step]
`)
	summary, err := NewRunner().Run(context.Background(), Options{Command: CommandCheck, Config: cfg, Profile: "fast", Workdir: project})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if got := summary.Results[0].Message; got != "project" {
		t.Fatalf("message = %q", got)
	}
}

// 验证完整 CLI 流程：用真实 fixture 项目跑 semantic 检测，输出包含 FAILED、规则ID、文件名和报告路径。
func TestGoSemanticFixtureViaCLI(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses go run shell path assumptions")
	}
	root := filepath.Dir(filepath.Dir(filepath.Dir(currentFile(t))))
	configPath := filepath.Join(root, "testdata/fixtures/regression-gates/configs/go-review.yaml")
	workdir := filepath.Join(root, "testdata/fixtures/regression-gates/semantic-violating-project")

	commands := []struct {
		name string
		args []string
	}{
		{name: "go-run", args: []string{"go", "run", "./cmd/go-review"}},
	}

	if runtime.GOOS == "darwin" && runtime.GOARCH == "arm64" {
		bin := filepath.Join(t.TempDir(), "go-review-trimpath")
		build := exec.Command("go", "build", "-trimpath", "-ldflags", "-s -w", "-o", bin, "./cmd/go-review")
		build.Dir = root
		if out, err := build.CombinedOutput(); err != nil {
			t.Fatalf("build trimpath binary: %v\n%s", err, out)
		}
		commands = append(commands, struct {
			name string
			args []string
		}{name: "trimpath-binary", args: []string{bin}})
	}

	for _, tc := range commands {
		t.Run(tc.name, func(t *testing.T) {
			args := append([]string{}, tc.args...)
			args = append(args, "check", "--config", configPath, "--profile", "semantic", "--workdir", workdir)
			cmd := exec.Command(args[0], args[1:]...)
			cmd.Dir = root
			out, err := cmd.CombinedOutput()
			if err == nil {
				t.Fatalf("semantic fixture should fail, output:\n%s", out)
			}
			for _, want := range []string{"FAILED profile=semantic", "semantic.no-direct-os-getenv", "main.go:", "report="} {
				if !strings.Contains(string(out), want) {
					t.Fatalf("semantic CLI output missing %q:\n%s", want, out)
				}
			}
		})
	}
}

func fakeGolangciLint(t *testing.T) string {
	t.Helper()
	return filepath.Join(filepath.Dir(currentFile(t)), "testdata", "bin", "fake-golangci-lint")
}

func currentFile(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	return file
}

// 验证 go.semantic adapter 能同时加载 default.yaml 和 custom.yaml，合并后的规则正确执行。
func TestSemanticAdapterLoadsDefaultAndCustomRuleFiles(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.com/semantic-config\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\nimport \"os\"\nfunc Message() string { return os.Getenv(\"MESSAGE\") }\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(dir, ".go-review"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, ".go-review", "semantic"), 0o755); err != nil {
		t.Fatal(err)
	}
	cfg := writeConfig(t, filepath.Join(dir, ".go-review"), `schema_version: "1.0"
defaults:
  workdir: ..
adapters:
  - id: semantic.rules
    type: go.semantic
    fix_safety: review
steps:
  - id: semantic-step
    adapter: semantic.rules
profiles:
  - name: fast
    steps: [semantic-step]
`)
	if err := os.WriteFile(filepath.Join(dir, ".go-review", "semantic", "default.yaml"), []byte("rules:\n  - no-direct-os-getenv\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".go-review", "semantic", "custom.yaml"), []byte("rules:\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	summary, err := NewRunner().Run(context.Background(), Options{Command: CommandCheck, Config: cfg, Profile: "fast"})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	got := summary.Results[0]
	if got.GateStatus != config.GateFail || got.RuleID != noDirectEnvRuleID || got.File != "main.go" {
		t.Fatalf("semantic config result = %#v", got)
	}
}

// 验证内置 semantic catalog 规则覆盖 B 类规则中的确定性 AST 检查。
func TestSemanticAdapterBuiltInCatalogRules(t *testing.T) {
	tests := []struct {
		name   string
		rule   string
		file   string
		source string
		wantID string
	}{
		{
			name:   "import blank",
			rule:   "import-blank",
			file:   "pkg.go",
			source: "package pkg\nimport _ \"net/http/pprof\"\n",
			wantID: "go.official.import-blank",
		},
		{
			name:   "custom contexts",
			rule:   "custom-contexts",
			file:   "context.go",
			source: "package pkg\ntype RequestContext interface { Done() <-chan struct{} }\n",
			wantID: "google.libs.custom-contexts",
		},
		{
			name:   "no tfatal goroutine",
			rule:   "no-tfatal-goroutine",
			file:   "main_test.go",
			source: "package pkg\nimport \"testing\"\nfunc TestX(t *testing.T) { go func() { t.Fatal(\"bad\") }() }\n",
			wantID: "google.bp.no-tfatal-goroutine",
		},
		{
			name:   "channel size",
			rule:   "channel-size",
			file:   "chan.go",
			source: "package pkg\nfunc f() { _ = make(chan int, 2) }\n",
			wantID: "uber.guideline.channel-size",
		},
		{
			name:   "enum start one",
			rule:   "enum-start-one",
			file:   "enum.go",
			source: "package pkg\nconst (\n\tReady = iota\n\tDone\n)\n",
			wantID: "uber.guideline.enum-start-one",
		},
		{
			name:   "exit in main",
			rule:   "exit-in-main",
			file:   "lib.go",
			source: "package pkg\nimport \"os\"\nfunc Die() { os.Exit(1) }\n",
			wantID: "uber.guideline.exit-in-main",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.com/semantic-builtin\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(dir, tt.file), []byte(tt.source), 0o644); err != nil {
				t.Fatal(err)
			}
			if err := os.MkdirAll(filepath.Join(dir, ".go-review", "semantic"), 0o755); err != nil {
				t.Fatal(err)
			}
			cfg := writeConfig(t, filepath.Join(dir, ".go-review"), `schema_version: "1.0"
defaults:
  workdir: ..
adapters:
  - id: semantic.rules
    type: go.semantic
    fix_safety: review
steps:
  - id: semantic-step
    adapter: semantic.rules
profiles:
  - name: fast
    steps: [semantic-step]
`)
			if err := os.WriteFile(filepath.Join(dir, ".go-review", "semantic", "default.yaml"), []byte("rules:\n  - "+tt.rule+"\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(dir, ".go-review", "semantic", "custom.yaml"), []byte("rules:\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			summary, err := NewRunner().Run(context.Background(), Options{Command: CommandCheck, Config: cfg, Profile: "fast"})
			if err != nil {
				t.Fatalf("Run() error = %v", err)
			}
			got := summary.Results[0]
			if got.GateStatus != config.GateFail || got.RuleID != tt.wantID || got.File != tt.file {
				t.Fatalf("semantic built-in result = %#v", got)
			}
		})
	}
}

// 验证 custom.yaml 的 rules/no-direct-call 规则能识别别名 import（如 import f "fmt"）并正确报告违规。
func TestSemanticAdapterLoadsCustomNoDirectCallRule(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.com/semantic-custom\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\nimport f \"fmt\"\nfunc main() { f.Println(\"debug\") }\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(dir, ".go-review"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, ".go-review", "semantic"), 0o755); err != nil {
		t.Fatal(err)
	}
	cfg := writeConfig(t, filepath.Join(dir, ".go-review"), `schema_version: "1.0"
defaults:
  workdir: ..
adapters:
  - id: semantic.rules
    type: go.semantic
    fix_safety: review
steps:
  - id: semantic-step
    adapter: semantic.rules
profiles:
  - name: fast
    steps: [semantic-step]
`)
	if err := os.WriteFile(filepath.Join(dir, ".go-review", "semantic", "default.yaml"), []byte("rules:\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".go-review", "semantic", "custom.yaml"), []byte(`rules:
  - id: no-direct-fmt-println
    kind: no-direct-call
    package: fmt
    function: Println
    message: 不要直接使用 fmt.Println
    suggestion: 改用注入的 logger
`), 0o644); err != nil {
		t.Fatal(err)
	}
	summary, err := NewRunner().Run(context.Background(), Options{Command: CommandCheck, Config: cfg, Profile: "fast"})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	got := summary.Results[0]
	if got.GateStatus != config.GateFail || got.RuleID != "semantic.no-direct-fmt-println" || got.File != "main.go" || got.Line == 0 {
		t.Fatalf("semantic custom rule result = %#v", got)
	}
	if !strings.Contains(got.Message, "不要直接使用") || got.Suggestion != "改用注入的 logger" {
		t.Fatalf("semantic custom rule message/suggestion = %#v", got)
	}
}

// 验证 custom.yaml 的 rules 配置了不支持的 kind 时，返回 fail 并提示 "unsupported custom rule kind"。
func TestSemanticAdapterCustomRuleConfigError(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.com/semantic-custom-error\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\nfunc main() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(dir, ".go-review"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, ".go-review", "semantic"), 0o755); err != nil {
		t.Fatal(err)
	}
	cfg := writeConfig(t, filepath.Join(dir, ".go-review"), `schema_version: "1.0"
defaults:
  workdir: ..
adapters:
  - id: semantic.rules
    type: go.semantic
steps:
  - id: semantic-step
    adapter: semantic.rules
profiles:
  - name: fast
    steps: [semantic-step]
`)
	if err := os.WriteFile(filepath.Join(dir, ".go-review", "semantic", "custom.yaml"), []byte(`rules:
  - id: unsupported
    kind: unknown
    package: fmt
    function: Println
`), 0o644); err != nil {
		t.Fatal(err)
	}
	summary, err := NewRunner().Run(context.Background(), Options{Command: CommandCheck, Config: cfg, Profile: "fast"})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	got := summary.Results[0]
	if got.GateStatus != config.GateFail || got.RuleID != "semantic.config" || !strings.Contains(got.Message, "unsupported custom rule kind") {
		t.Fatalf("semantic custom config error = %#v", got)
	}
}

// 验证 go.semantic 不再兼容 adapter parser 字段；内置规则必须写在 semantic/default.yaml。
func TestSemanticAdapterRejectsAdapterParserField(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\nfunc main() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg := writeConfig(t, dir, `schema_version: "1.0"
defaults:
  workdir: .
adapters:
  - id: semantic.rules
    type: go.semantic
    parser: no-direct-os-getenv
steps:
  - id: semantic-step
    adapter: semantic.rules
profiles:
  - name: fast
    steps: [semantic-step]
`)
	summary, err := NewRunner().Run(context.Background(), Options{Command: CommandCheck, Config: cfg, Profile: "fast"})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	got := summary.Results[0]
	if got.GateStatus != config.GateFail || got.RuleID != "semantic.config" || !strings.Contains(got.Message, "does not support adapter parser") {
		t.Fatalf("semantic parser rejection result = %#v", got)
	}
}

// 验证 default.yaml 不支持未知内置规则名。
func TestSemanticAdapterUnsupportedBuiltInRuleIsConfigError(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\nfunc main() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "semantic"), 0o755); err != nil {
		t.Fatal(err)
	}
	cfg := writeConfig(t, dir, `schema_version: "1.0"
defaults:
  workdir: .
adapters:
  - id: semantic.rules
    type: go.semantic
steps:
  - id: semantic-step
    adapter: semantic.rules
profiles:
  - name: fast
    steps: [semantic-step]
`)
	if err := os.WriteFile(filepath.Join(dir, "semantic", "default.yaml"), []byte("rules:\n  - unknown-rule\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	summary, err := NewRunner().Run(context.Background(), Options{Command: CommandCheck, Config: cfg, Profile: "fast"})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	got := summary.Results[0]
	if got.GateStatus != config.GateFail || got.RuleID != "semantic.config" || !strings.Contains(got.Message, `unsupported semantic rule "unknown-rule"`) {
		t.Fatalf("unsupported built-in rule result = %#v", got)
	}
}

// 验证 semantic rules 目前只报告首个 finding，避免误以为单 step 会输出多 diagnostics。
func TestSemanticAdapterReportsFirstFindingOnly(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.com/semantic-first\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "a.go"), []byte("package main\nimport \"os\"\nfunc A() string { return os.Getenv(\"A\") }\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "b.go"), []byte("package main\nimport \"os\"\nfunc B() string { return os.Getenv(\"B\") }\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "semantic"), 0o755); err != nil {
		t.Fatal(err)
	}
	cfg := writeConfig(t, dir, `schema_version: "1.0"
defaults:
  workdir: .
adapters:
  - id: semantic.rules
    type: go.semantic
    fix_safety: review
steps:
  - id: semantic-step
    adapter: semantic.rules
profiles:
  - name: fast
    steps: [semantic-step]
`)
	if err := os.WriteFile(filepath.Join(dir, "semantic", "default.yaml"), []byte("rules:\n  - no-direct-os-getenv\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	summary, err := NewRunner().Run(context.Background(), Options{Command: CommandCheck, Config: cfg, Profile: "fast"})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if len(summary.Results) != 1 {
		t.Fatalf("semantic results = %d", len(summary.Results))
	}
	got := summary.Results[0]
	if got.GateStatus != config.GateFail || got.File != "a.go" || got.RuleID != noDirectEnvRuleID {
		t.Fatalf("first semantic finding = %#v", got)
	}
}

// 验证 custom.yaml 的 rules 支持 pkg/func 字段别名。
func TestSemanticAdapterRuleScalarAndCustomPkgFuncAliases(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.com/semantic-alias-fields\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\nimport log \"log\"\nfunc main() { log.Fatal(\"stop\") }\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, ".go-review", "semantic"), 0o755); err != nil {
		t.Fatal(err)
	}
	cfg := writeConfig(t, filepath.Join(dir, ".go-review"), `schema_version: "1.0"
defaults:
  workdir: ..
adapters:
  - id: semantic.rules
    type: go.semantic
steps:
  - id: semantic-step
    adapter: semantic.rules
profiles:
  - name: fast
    steps: [semantic-step]
`)
	if err := os.WriteFile(filepath.Join(dir, ".go-review", "semantic", "default.yaml"), []byte("rules: no-direct-os-getenv\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".go-review", "semantic", "custom.yaml"), []byte(`rules:
  - id: no-log-fatal
    pkg: log
    func: Fatal
`), 0o644); err != nil {
		t.Fatal(err)
	}
	summary, err := NewRunner().Run(context.Background(), Options{Command: CommandCheck, Config: cfg, Profile: "fast"})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	got := summary.Results[0]
	if got.GateStatus != config.GateFail || got.RuleID != "semantic.no-log-fatal" || got.File != "main.go" {
		t.Fatalf("custom pkg/func alias result = %#v", got)
	}
}

// 验证 custom.yaml 的 rules/max-params 规则能用 go/analysis 检测函数/方法参数数量上限。
func TestSemanticAdapterLoadsCustomMaxParamsRule(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.com/semantic-max-params\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\ntype service struct{}\nfunc ok(a, b int, c string, d bool) {}\nfunc tooMany(a, b int, c string, d bool, e error) {}\nfunc (service) method(a, b, c, d, e int) {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, ".go-review", "semantic"), 0o755); err != nil {
		t.Fatal(err)
	}
	cfg := writeConfig(t, filepath.Join(dir, ".go-review"), `schema_version: "1.0"
defaults:
  workdir: ..
adapters:
  - id: semantic.rules
    type: go.semantic
    fix_safety: review
steps:
  - id: semantic-step
    adapter: semantic.rules
profiles:
  - name: fast
    steps: [semantic-step]
`)
	if err := os.WriteFile(filepath.Join(dir, ".go-review", "semantic", "default.yaml"), []byte("rules:\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".go-review", "semantic", "custom.yaml"), []byte(`rules:
  - id: team.semantic.max-params
    kind: max-params
    max: 4
    message: 方法入参不能超过 4 个
    suggestion: 拆分参数对象或引入配置结构
`), 0o644); err != nil {
		t.Fatal(err)
	}
	summary, err := NewRunner().Run(context.Background(), Options{Command: CommandCheck, Config: cfg, Profile: "fast"})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	got := summary.Results[0]
	if got.GateStatus != config.GateFail || got.RuleID != "team.semantic.max-params" || got.File != "main.go" || got.Line == 0 {
		t.Fatalf("semantic max-params result = %#v", got)
	}
	if !strings.Contains(got.Message, "不能超过 4") || got.Suggestion != "拆分参数对象或引入配置结构" {
		t.Fatalf("semantic max-params message/suggestion = %#v", got)
	}
}

// 验证 max-params 规则在未超限时通过。
func TestSemanticAdapterCustomMaxParamsRulePassesAtBoundary(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.com/semantic-max-params-pass\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\nfunc ok(a, b int, c string, d bool) {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, ".go-review", "semantic"), 0o755); err != nil {
		t.Fatal(err)
	}
	cfg := writeConfig(t, filepath.Join(dir, ".go-review"), `schema_version: "1.0"
defaults:
  workdir: ..
adapters:
  - id: semantic.rules
    type: go.semantic
steps:
  - id: semantic-step
    adapter: semantic.rules
profiles:
  - name: fast
    steps: [semantic-step]
`)
	if err := os.WriteFile(filepath.Join(dir, ".go-review", "semantic", "default.yaml"), []byte("rules:\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".go-review", "semantic", "custom.yaml"), []byte(`rules:
  - id: team.semantic.max-params
    kind: max-params
    max: 4
`), 0o644); err != nil {
		t.Fatal(err)
	}
	summary, err := NewRunner().Run(context.Background(), Options{Command: CommandCheck, Config: cfg, Profile: "fast"})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	got := summary.Results[0]
	if got.GateStatus != config.GatePass || got.RuleID != "semantic.rules" {
		t.Fatalf("semantic max-params boundary result = %#v", got)
	}
}

// 验证 semantic 配置解析错误统一归入 semantic.config。
func TestSemanticAdapterConfigValidationErrors(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    string
	}{
		{name: "unsupported top-level field", content: "unknown: true\n", want: "unsupported semantic config field"},
		{name: "legacy custom_rules field", content: "custom_rules:\n  - id: legacy\n    package: fmt\n    function: Println\n", want: "unsupported semantic config field"},
		{name: "unsupported custom field", content: "rules:\n  - id: bad\n    package: fmt\n    function: Println\n    severity: high\n", want: "unsupported custom rule field"},
		{name: "missing max", content: "rules:\n  - id: max-params\n    kind: max-params\n", want: "requires positive max"},
		{name: "invalid max", content: "rules:\n  - id: max-params\n    kind: max-params\n    max: nope\n", want: "max must be a positive integer"},
		{name: "missing id", content: "rules:\n  - package: fmt\n    function: Println\n", want: "custom rule missing id"},
		{name: "missing package", content: "rules:\n  - id: missing-package\n    function: Println\n", want: "requires package and function"},
		{name: "unsupported custom kind", content: "rules:\n  - id: bad-kind\n    kind: unknown\n    package: fmt\n    function: Println\n", want: "unsupported custom rule kind"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.com/semantic-config-error\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\nfunc main() {}\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			if err := os.MkdirAll(filepath.Join(dir, ".go-review", "semantic"), 0o755); err != nil {
				t.Fatal(err)
			}
			cfg := writeConfig(t, filepath.Join(dir, ".go-review"), `schema_version: "1.0"
defaults:
  workdir: ..
adapters:
  - id: semantic.rules
    type: go.semantic
steps:
  - id: semantic-step
    adapter: semantic.rules
profiles:
  - name: fast
    steps: [semantic-step]
`)
			if err := os.WriteFile(filepath.Join(dir, ".go-review", "semantic", "default.yaml"), []byte("rules:\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(dir, ".go-review", "semantic", "custom.yaml"), []byte(tt.content), 0o644); err != nil {
				t.Fatal(err)
			}
			summary, err := NewRunner().Run(context.Background(), Options{Command: CommandCheck, Config: cfg, Profile: "fast"})
			if err != nil {
				t.Fatalf("Run() error = %v", err)
			}
			got := summary.Results[0]
			if got.GateStatus != config.GateFail || got.RuleID != "semantic.config" || !strings.Contains(got.Message, tt.want) {
				t.Fatalf("semantic config validation result = %#v, want %q", got, tt.want)
			}
		})
	}
}

// 验证 type-check 失败时 semantic 规则降级到 import alias 匹配，不直接把 type-check error 作为 gate fail。
func TestSemanticAdapterTypeCheckErrorFallsBackToImportAlias(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.com/semantic-typecheck-fallback\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\nimport f \"fmt\"\nvar _ MissingType\nfunc main() { f.Println(\"debug\") }\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, ".go-review", "semantic"), 0o755); err != nil {
		t.Fatal(err)
	}
	cfg := writeConfig(t, filepath.Join(dir, ".go-review"), `schema_version: "1.0"
defaults:
  workdir: ..
adapters:
  - id: semantic.rules
    type: go.semantic
steps:
  - id: semantic-step
    adapter: semantic.rules
profiles:
  - name: fast
    steps: [semantic-step]
`)
	if err := os.WriteFile(filepath.Join(dir, ".go-review", "semantic", "default.yaml"), []byte("rules:\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".go-review", "semantic", "custom.yaml"), []byte(`rules:
  - id: no-direct-fmt-println
    kind: no-direct-call
    package: fmt
    function: Println
`), 0o644); err != nil {
		t.Fatal(err)
	}
	summary, err := NewRunner().Run(context.Background(), Options{Command: CommandCheck, Config: cfg, Profile: "fast"})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	got := summary.Results[0]
	if got.GateStatus != config.GateFail || got.RuleID != "semantic.no-direct-fmt-println" || !strings.Contains(got.Message, "fmt.Println") {
		t.Fatalf("semantic type-check fallback result = %#v", got)
	}
}

// 验证 go-review.yaml 中 exclude 配置的目录会被 semantic adapter 跳过，不触发违规。
func TestProjectExcludeAppliesToSemanticAdapter(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.com/semantic-exclude\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	excludedDir := filepath.Join(dir, "fixtures")
	if err := os.Mkdir(excludedDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(excludedDir, "main.go"), []byte("package fixtures\nimport \"os\"\nfunc Message() string { return os.Getenv(\"MESSAGE\") }\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(dir, ".go-review"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, ".go-review", "semantic"), 0o755); err != nil {
		t.Fatal(err)
	}
	cfg := writeConfig(t, filepath.Join(dir, ".go-review"), `schema_version: "1.0"
defaults:
  workdir: ..
exclude: [fixtures]
adapters:
  - id: semantic.rules
    type: go.semantic
    fix_safety: review
steps:
  - id: semantic-step
    adapter: semantic.rules
profiles:
  - name: fast
    steps: [semantic-step]
`)
	if err := os.WriteFile(filepath.Join(dir, ".go-review", "semantic", "default.yaml"), []byte("rules: [no-direct-os-getenv]\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".go-review", "semantic", "custom.yaml"), []byte("rules:\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	summary, err := NewRunner().Run(context.Background(), Options{Command: CommandCheck, Config: cfg, Profile: "fast"})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	got := summary.Results[0]
	if got.GateStatus != config.GatePass {
		t.Fatalf("semantic exclude result = %#v", got)
	}
}

// 验证 go-review.yaml 中 exclude 配置的目录会被 go.lint adapter 跳过，不触发格式检查。
func TestProjectExcludeAppliesToGoLintAdapter(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.com/format-exclude\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	generated := filepath.Join(dir, "generated")
	if err := os.Mkdir(generated, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(generated, "bad.go"), []byte("package generated\nfunc Bad(){println(\"skip\")}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg := writeConfig(t, dir, fmt.Sprintf(`schema_version: "1.0"
defaults:
  workdir: .
exclude:
  - generated
adapters:
  - id: format
    type: go.lint
    command: %s
    fix_safety: safe
steps:
  - id: format-check
    adapter: format
profiles:
  - name: fast
    steps: [format-check]
`, fakeGolangciLint(t)))
	summary, err := NewRunner().Run(context.Background(), Options{Command: CommandCheck, Config: cfg, Profile: "fast"})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	got := summary.Results[0]
	if got.GateStatus != config.GatePass {
		t.Fatalf("format exclude result = %#v", got)
	}
}
