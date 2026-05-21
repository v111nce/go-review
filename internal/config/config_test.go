package config

import (
	"strings"
	"testing"
	"time"
)

func TestLoadValidConfig(t *testing.T) {
	cfg, err := Load(strings.NewReader(`
schema_version: "1.0"
tools:
  go_review: "0.1.0"
  adapters:
    cmd: "1"
defaults:
  timeout: 2s
  workdir: .
adapters:
  - id: echo
    type: cmd
    command: sh
    args: [-c, "echo ok"]
    env:
      REVIEW_ENV: test
    capabilities: [check]
    fix_safety: none
steps:
  - id: echo-step
    adapter: echo
    on_fail: continue
profiles:
  - name: fast
    steps: [echo-step]
artifacts:
  dir: .go-review/artifacts
`))
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.SchemaVersion != "1.0" {
		t.Fatalf("schema version = %q", cfg.SchemaVersion)
	}
	if cfg.Defaults.Timeout != 2*time.Second {
		t.Fatalf("timeout = %v", cfg.Defaults.Timeout)
	}
	adapter, ok := cfg.Adapter("echo")
	if !ok || adapter.Command != "sh" || adapter.Env["REVIEW_ENV"] != "test" {
		t.Fatalf("adapter not parsed: %#v", adapter)
	}
	profile, err := cfg.Profile("fast")
	if err != nil {
		t.Fatalf("Profile() error = %v", err)
	}
	if got := profile.Steps[0]; got != "echo-step" {
		t.Fatalf("profile step = %q", got)
	}
}

func TestUnsupportedSchemaMajorFails(t *testing.T) {
	_, err := Load(strings.NewReader(`
schema_version: "2.0"
`))
	if err == nil {
		t.Fatal("expected schema validation error")
	}
}

func TestUnsupportedSchemaMinorFails(t *testing.T) {
	_, err := Load(strings.NewReader(`
schema_version: "1.1"
`))
	if err == nil {
		t.Fatal("expected schema validation error")
	}
}

func TestParseEnums(t *testing.T) {
	if got, _ := ParseFixSafety("manual-review"); got != FixReview {
		t.Fatalf("manual-review = %q", got)
	}
	if got, _ := ParseFixSafety("report-only"); got != FixNone {
		t.Fatalf("report-only = %q", got)
	}
	if got, _ := ParseGateStatus("fail"); got != GateFail {
		t.Fatalf("fail = %q", got)
	}
	if got, _ := ParseOnFail(""); got != OnFailStop {
		t.Fatalf("empty on_fail = %q", got)
	}
}

func TestValidationRejectsUnknownProfileStep(t *testing.T) {
	_, err := Load(strings.NewReader(`
schema_version: "1.0"
adapters:
  - id: echo
    type: cmd
steps:
  - id: known
    adapter: echo
profiles:
  - name: fast
    steps: [missing]
`))
	if err == nil {
		t.Fatal("expected unknown step error")
	}
}
