package engine

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/v111nce/go-review/internal/config"
)

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
  - name: local
    steps: [ok-step, fail-step]
artifacts:
  dir: artifacts
`)
	summary, err := NewRunner().Run(context.Background(), Options{Command: CommandCheck, Config: cfg, Profile: "local"})
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
  - name: local
    steps: [slow-step]
`)
	summary, err := NewRunner().Run(context.Background(), Options{Command: CommandCheck, Config: cfg, Profile: "local"})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if got := summary.Results[0].GateStatus; got != config.GateFail {
		t.Fatalf("timeout gate = %s", got)
	}
}

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
  - name: local
    steps: [env-step]
`)
	summary, err := NewRunner().Run(context.Background(), Options{Command: CommandCheck, Config: cfg, Profile: "local"})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if got := summary.Results[0].Message; got != "ok:sub" {
		t.Fatalf("message = %q", got)
	}
}

func TestGoFormatCheckDetectsUnformattedFile(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "bad.go"), []byte("package main\nfunc main(){\n}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg := writeConfig(t, dir, `schema_version: "1.0"
defaults:
  workdir: .
adapters:
  - id: format
    type: go.format
    args: [-l, .]
    fix_safety: safe
steps:
  - id: format-step
    adapter: format
profiles:
  - name: local
    steps: [format-step]
`)
	summary, err := NewRunner().Run(context.Background(), Options{Command: CommandCheck, Config: cfg, Profile: "local"})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if got := summary.Results[0].GateStatus; got != config.GateFail {
		t.Fatalf("format gate = %s", got)
	}
	if !summary.Results[0].FixAvailable {
		t.Fatal("expected fix availability")
	}
}

func TestGoFormatFixRequiresSafeAllowedStep(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "bad.go")
	if err := os.WriteFile(file, []byte("package main\nfunc main(){\n}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg := writeConfig(t, dir, `schema_version: "1.0"
defaults:
  workdir: .
adapters:
  - id: format
    type: go.format
    fix_safety: review
steps:
  - id: format-step
    adapter: format
    allow_fix: true
profiles:
  - name: local
    steps: [format-step]
`)
	summary, err := NewRunner().Run(context.Background(), Options{Command: CommandFix, Config: cfg, Profile: "local"})
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

func TestFixRollsBackWhenDependentValidationFails(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "bad.go"), []byte("package main\nfunc main(){println(Message())}\nfunc Message() string { return \"wrong\" }\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "bad_test.go"), []byte("package main\nimport \"testing\"\nfunc TestMessage(t *testing.T) { if Message() != \"right\" { t.Fatal(\"validation failed\") } }\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg := writeConfig(t, dir, `schema_version: "1.0"
defaults:
  workdir: .
adapters:
  - id: format
    type: go.format
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
  - name: local
    steps: [format-step, test-step]
`)
	summary, err := NewRunner().Run(context.Background(), Options{Command: CommandFix, Config: cfg, Profile: "local"})
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
    parser: no-direct-os-getenv
    fix_safety: review
steps:
  - id: semantic-step
    adapter: semantic.no-env
profiles:
  - name: local
    steps: [semantic-step]
`)
	summary, err := NewRunner().Run(context.Background(), Options{Command: CommandCheck, Config: cfg, Profile: "local"})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	got := summary.Results[0]
	if got.GateStatus != config.GateFail || got.RuleID != noDirectEnvRuleID || got.File != "main.go" || got.Line == 0 || got.Suggestion == "" {
		t.Fatalf("semantic result = %#v", got)
	}
}

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
  - name: local
    steps: [fail-step, ok-step]
`)
	summary, err := NewRunner().Run(context.Background(), Options{Command: CommandCheck, Config: cfg, Profile: "local"})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if len(summary.Results) != 1 {
		t.Fatalf("results = %d", len(summary.Results))
	}
}

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
  - name: local
    steps: [second-step, first-step]
`)
	summary, err := NewRunner().Run(context.Background(), Options{Command: CommandCheck, Config: cfg, Profile: "local"})
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

func TestRegistryRejectsUnknownAdapter(t *testing.T) {
	_, err := NewRegistry().Resolve(config.Adapter{ID: "x", Type: "missing"})
	if err == nil {
		t.Fatal("expected unknown adapter error")
	}
}

func TestTimeoutFromStepOverridesAdapter(t *testing.T) {
	r := CommandAdapter{cfg: config.Adapter{ID: "slow", Command: "sh", Args: []string{"-c", "sleep 1"}, Timeout: time.Second}}
	result, _ := r.Run(context.Background(), StepContext{Step: config.Step{ID: "s", Timeout: time.Millisecond}, Adapter: config.Adapter{ID: "slow"}, Config: &config.Config{}, ProjectRoot: "."})
	if result.GateStatus != config.GateFail {
		t.Fatalf("gate = %s", result.GateStatus)
	}
}

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
  - name: local
    steps: [pwd-step]
`)
	summary, err := NewRunner().Run(context.Background(), Options{Command: CommandCheck, Config: cfg, Profile: "local", Workdir: project})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if got := summary.Results[0].Message; got != "project" {
		t.Fatalf("message = %q", got)
	}
}

func TestGoSemanticFixtureViaCLI(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses go run shell path assumptions")
	}
	root := filepath.Dir(filepath.Dir(filepath.Dir(currentFile(t))))
	configPath := filepath.Join(root, "testdata/fixtures/regression-gates/configs/go-review.yaml")
	workdir := filepath.Join(root, "testdata/fixtures/regression-gates/semantic-violating-project")
	cmd := exec.Command("go", "run", "./cmd/go-review", "check", "--config", configPath, "--profile", "semantic", "--workdir", workdir)
	cmd.Dir = root
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("semantic fixture should fail, output:\n%s", out)
	}
	for _, want := range []string{"semantic.no-direct-os-getenv", "main.go:", "fix_safety=review"} {
		if !strings.Contains(string(out), want) {
			t.Fatalf("semantic CLI output missing %q:\n%s", want, out)
		}
	}
}

func currentFile(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	return file
}
