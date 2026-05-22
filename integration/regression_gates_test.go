package integration_test

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/v111nce/go-review/internal/config"
	"github.com/v111nce/go-review/internal/report"
)

func repoRoot(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	return filepath.Dir(wd)
}

func TestRegressionGateFixtureConfigProfiles(t *testing.T) {
	root := repoRoot(t)
	cfg, err := config.LoadFile(filepath.Join(root, "testdata/fixtures/regression-gates/configs/go-review.yaml"))
	if err != nil {
		t.Fatalf("LoadFile fixture config: %v", err)
	}

	cases := map[string][]string{
		"fast":     {"format-check", "test"},
		"ci":       {"format-check", "test", "security-lite"},
		"main":     {"format-check", "test", "security-lite"},
		"nightly":  {"format-check", "test", "security-lite", "full-security", "custom-long-rules", "semantic-no-env"},
		"semantic": {"semantic-no-env"},
	}
	for profile, want := range cases {
		t.Run(profile, func(t *testing.T) {
			got, err := cfg.Profile(profile)
			if err != nil {
				t.Fatalf("Profile(%q): %v", profile, err)
			}
			if strings.Join(got.Steps, ",") != strings.Join(want, ",") {
				t.Fatalf("steps = %v, want %v", got.Steps, want)
			}
		})
	}

	format, ok := cfg.Adapter("go.lint")
	if !ok {
		t.Fatal("go.lint adapter missing")
	}
	if format.FixSafety != config.FixSafe {
		t.Fatalf("go.lint fix safety = %q, want safe", format.FixSafety)
	}
}

func TestRegressionGateSemanticAndAutofixFixtures(t *testing.T) {
	root := repoRoot(t)
	configPath := filepath.Join(root, "testdata/fixtures/regression-gates/configs/go-review.yaml")

	semanticProject := filepath.Join(root, "testdata/fixtures/regression-gates/semantic-violating-project")
	semantic := command("go", "run", "./cmd/go-review", "check", "--config", configPath, "--profile", "semantic", "--workdir", semanticProject).WithDir(root)
	if out, err := semantic.CombinedOutput(); err == nil || !strings.Contains(string(out), "semantic.no-direct-os-getenv") {
		t.Fatalf("semantic fixture should fail with normalized rule, err=%v out=%s", err, out)
	}

	autofixProject := filepath.Join(root, "testdata/fixtures/regression-gates/autofix-project")
	fix := command("go", "run", "./cmd/go-review", "fix", "--config", configPath, "--profile", "fast", "--workdir", autofixProject).WithDir(root)
	if out, err := fix.CombinedOutput(); err != nil {
		t.Fatalf("autofix fixture should pass after safe golangci-lint fmt and validation: %v\n%s", err, out)
	}
	defer func() {
		_ = os.WriteFile(filepath.Join(autofixProject, "main.go"), []byte("package main\n\nimport \"fmt\"\n\nfunc main(){fmt.Println(Message())}\n\nfunc Message() string { return \"autofix fixture\" }\n"), 0o644)
	}()
	if out, err := command("../scripts/fake-golangci-lint", "fmt", "--diff", "main.go").WithDir(autofixProject).CombinedOutput(); err != nil || strings.TrimSpace(string(out)) != "" {
		t.Fatalf("autofix fixture should be formatted, err=%v out=%q", err, out)
	}
}

func TestRegressionGateFixtureProjects(t *testing.T) {
	root := repoRoot(t)
	compliant := filepath.Join(root, "testdata/fixtures/regression-gates/compliant-project")
	if out, err := command("go", "test", "./...").WithDir(compliant).CombinedOutput(); err != nil {
		t.Fatalf("compliant fixture should pass go test: %v\n%s", err, out)
	}

	violating := filepath.Join(root, "testdata/fixtures/regression-gates/violating-project")
	if out, err := command("../scripts/fake-golangci-lint", "fmt", "--diff", "main.go").WithDir(violating).CombinedOutput(); err == nil || !strings.Contains(string(out), "main.go") {
		t.Fatalf("violating fixture should be reported by golangci-lint fmt, err=%v out=%q", err, out)
	}
	if out, err := command("go", "test", "./...").WithDir(violating).CombinedOutput(); err == nil || !strings.Contains(string(out), "intentional failure") {
		t.Fatalf("violating fixture should fail go test with intentional failure, err=%v out=%s", err, out)
	}
}

func TestGoldenReportContract(t *testing.T) {
	r := report.RunReport{
		Profile:    "fast",
		GateStatus: report.GatePass,
		Steps: []report.Step{
			{ID: "format-check", AdapterID: "go.lint", Status: report.GatePass},
			{ID: "test", AdapterID: "go.test", Status: report.GatePass},
		},
	}
	var buf bytes.Buffer
	if err := report.WriteTerminal(&buf, r); err != nil {
		t.Fatalf("WriteTerminal: %v", err)
	}
	got := normalizeTerminal(buf.String())
	wantBytes, err := os.ReadFile(filepath.Join(repoRoot(t), "testdata/fixtures/regression-gates/expected-reports/fast-pass.golden.txt"))
	if err != nil {
		t.Fatal(err)
	}
	want := normalizeTerminal(string(wantBytes))
	if got != want {
		t.Fatalf("terminal report mismatch\ngot:\n%s\nwant:\n%s", got, want)
	}
}

func normalizeTerminal(s string) string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	for i, line := range lines {
		line = strings.ReplaceAll(line, "[pass]", "PASS")
		line = strings.ReplaceAll(line, "[fail]", "FAIL")
		line = strings.ReplaceAll(line, " findings=0", "")
		lines[i] = strings.TrimSpace(line)
	}
	return strings.Join(lines, "\n") + "\n"
}

type dirCommand struct{ *exec.Cmd }

func (c *dirCommand) WithDir(dir string) *exec.Cmd {
	c.Cmd.Dir = dir
	return c.Cmd
}

func command(name string, arg ...string) *dirCommand { return &dirCommand{exec.Command(name, arg...)} }
