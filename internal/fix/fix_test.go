package fix

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestParseSafetyAliases(t *testing.T) {
	cases := map[string]Safety{
		"safe":          SafetySafe,
		"manual-review": SafetyReview,
		"report-only":   SafetyNone,
		"":              SafetyNone,
	}
	for input, want := range cases {
		got, err := ParseSafety(input)
		if err != nil {
			t.Fatalf("ParseSafety(%q) error = %v", input, err)
		}
		if got != want {
			t.Fatalf("ParseSafety(%q) = %s, want %s", input, got, want)
		}
	}
}

func TestPolicyDecisionPrecedence(t *testing.T) {
	policy := Policy{
		Default: SafetyNone,
		Profile: SafetyReview,
		Adapter: map[string]Safety{"go.format": SafetySafe},
		Rule:    map[string]Safety{"no-env": SafetyNone},
	}
	if got := policy.Decision("go.format", "other", ""); got != SafetySafe {
		t.Fatalf("adapter decision = %s, want safe", got)
	}
	if got := policy.Decision("go.format", "no-env", ""); got != SafetyNone {
		t.Fatalf("rule decision = %s, want none", got)
	}
	if got := policy.Decision("go.format", "no-env", SafetyReview); got != SafetyReview {
		t.Fatalf("result decision = %s, want review", got)
	}
}

func TestValidateEditsRejectsOverlaps(t *testing.T) {
	err := ValidateEdits([]TextEdit{
		{File: "a.go", Start: 0, End: 3, NewText: "abc"},
		{File: "a.go", Start: 2, End: 5, NewText: "def"},
	})
	if err == nil {
		t.Fatal("expected overlap error")
	}
}

func TestApplyToBytes(t *testing.T) {
	got, err := ApplyToBytes([]byte("hello world"), []TextEdit{
		{File: "a.go", Start: 6, End: 11, NewText: "gopher"},
		{File: "a.go", Start: 0, End: 5, NewText: "hi"},
	})
	if err != nil {
		t.Fatalf("ApplyToBytes() error = %v", err)
	}
	if string(got) != "hi gopher" {
		t.Fatalf("content = %q", got)
	}
}

func TestTransactionApplyRollbackOnValidatorFailure(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "a.go")
	if err := os.WriteFile(file, []byte("package main\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	tx := Transaction{
		Root: dir,
		Validator: func(paths []string) error {
			return errors.New("tests failed")
		},
	}
	result := tx.Apply(SafetySafe, []TextEdit{{File: "a.go", Start: 8, End: 12, NewText: "demo"}})
	if result.Applied || !result.RolledBack || result.FailureReason == "" {
		t.Fatalf("result = %#v, want rollback failure", result)
	}
	data, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "package main\n" {
		t.Fatalf("rollback content = %q", data)
	}
}

func TestTransactionRefusesNonSafe(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "a.go")
	if err := os.WriteFile(file, []byte("abc"), 0o644); err != nil {
		t.Fatal(err)
	}
	result := (Transaction{Root: dir}).Apply(SafetyReview, []TextEdit{{File: "a.go", Start: 0, End: 1, NewText: "z"}})
	if result.Applied || result.FailureReason == "" {
		t.Fatalf("result = %#v, want non-safe no-op", result)
	}
	data, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "abc" {
		t.Fatalf("non-safe changed file to %q", data)
	}
}
