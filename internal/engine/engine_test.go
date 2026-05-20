package engine

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"go-code-reviewer/internal/config"
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
	if summary.Results[0].GateStatus != config.GatePass {
		t.Fatalf("ok gate = %s", summary.Results[0].GateStatus)
	}
	if summary.Results[1].GateStatus != config.GateFail {
		t.Fatalf("fail gate = %s", summary.Results[1].GateStatus)
	}
	if summary.Results[0].Artifacts[0].Path == "" {
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
