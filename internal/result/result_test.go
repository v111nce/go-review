package result

import "testing"

func TestParseFixSafetyAliases(t *testing.T) {
	tests := map[string]FixSafety{
		"safe":          FixSafe,
		"manual-review": FixReview,
		"review":        FixReview,
		"report-only":   FixNone,
		"":              FixNone,
	}
	for raw, want := range tests {
		got, err := ParseFixSafety(raw)
		if err != nil {
			t.Fatalf("ParseFixSafety(%q) unexpected error: %v", raw, err)
		}
		if got != want {
			t.Fatalf("ParseFixSafety(%q) = %q, want %q", raw, got, want)
		}
	}
}

func TestResultNormalizeAndValidate(t *testing.T) {
	r := Result{AdapterID: "cmd", StepID: "lint", Message: "failed"}.Normalize()
	if r.Kind != KindViolation || r.Category != CategoryUnknown || r.Severity != SeverityUnknown || r.Scope != ScopeUnknown || r.GateStatus != GateFail || r.Fix.Safety != FixNone {
		t.Fatalf("Normalize produced unexpected defaults: %#v", r)
	}
	if err := r.Validate(); err != nil {
		t.Fatalf("Validate unexpected error: %v", err)
	}
}

func TestResultValidateRequiresCoreFields(t *testing.T) {
	r := Result{Kind: KindViolation, Category: CategoryUnknown, Severity: SeverityUnknown, Scope: ScopeUnknown, GateStatus: GateFail, Fix: Fix{Safety: FixNone}}
	if err := r.Validate(); err == nil {
		t.Fatal("Validate() expected error")
	}
}
