// Package result defines the normalized review output model shared by adapters,
// pipeline execution, fix policy, and report writers.
package result

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

// Kind describes the normalized record class emitted by an adapter.
type Kind string

const (
	KindViolation Kind = "violation"
	KindTest      Kind = "test"
	KindCoverage  Kind = "coverage"
	KindSecurity  Kind = "security"
	KindArtifact  Kind = "artifact"
)

// Severity describes how urgent a result is for humans and gates.
type Severity string

const (
	SeverityUnknown  Severity = "unknown"
	SeverityInfo     Severity = "info"
	SeverityLow      Severity = "low"
	SeverityMedium   Severity = "medium"
	SeverityHigh     Severity = "high"
	SeverityCritical Severity = "critical"
)

// Category groups results by review domain without binding the core to a tool.
type Category string

const (
	CategoryUnknown      Category = "unknown"
	CategoryFormat       Category = "format"
	CategoryLint         Category = "lint"
	CategoryArchitecture Category = "architecture"
	CategorySecurity     Category = "security"
	CategoryTest         Category = "test"
	CategoryCoverage     Category = "coverage"
	CategorySemantic     Category = "semantic"
	CategoryCommand      Category = "command"
)

// Scope identifies the affected review scope.
type Scope string

const (
	ScopeUnknown    Scope = "unknown"
	ScopeRepository Scope = "repository"
	ScopePackage    Scope = "package"
	ScopeFile       Scope = "file"
	ScopeLine       Scope = "line"
	ScopeSymbol     Scope = "symbol"
)

// GateStatus is the value consumed by pipeline/report gates.
type GateStatus string

const (
	GatePass GateStatus = "pass"
	GateWarn GateStatus = "warn"
	GateFail GateStatus = "fail"
)

// FixSafety is the canonical automatic-fix safety vocabulary.
type FixSafety string

const (
	FixSafe   FixSafety = "safe"
	FixReview FixSafety = "review"
	FixNone   FixSafety = "none"
)

// Location points at a source position when one is available.
type Location struct {
	File   string `json:"file,omitempty"`
	Line   int    `json:"line,omitempty"`
	Column int    `json:"column,omitempty"`
}

// Fix describes whether and how a result can be remediated.
type Fix struct {
	Available bool      `json:"available"`
	Safety    FixSafety `json:"safety"`
	Message   string    `json:"message,omitempty"`
}

// ArtifactRef preserves raw or derived adapter outputs for reports/debugging.
type ArtifactRef struct {
	Name        string `json:"name"`
	Kind        string `json:"kind,omitempty"`
	Path        string `json:"path,omitempty"`
	ContentType string `json:"content_type,omitempty"`
	Inline      string `json:"inline,omitempty"`
}

// Result is the normalized, tool-agnostic record produced by adapters.
type Result struct {
	AdapterID  string            `json:"adapter_id"`
	StepID     string            `json:"step_id"`
	RuleID     string            `json:"rule_id,omitempty"`
	Kind       Kind              `json:"kind"`
	Category   Category          `json:"category"`
	Severity   Severity          `json:"severity"`
	Scope      Scope             `json:"scope"`
	Location   Location          `json:"location,omitempty"`
	Message    string            `json:"message"`
	Suggestion string            `json:"suggestion,omitempty"`
	Fix        Fix               `json:"fix"`
	GateStatus GateStatus        `json:"gate_status"`
	Artifacts  []ArtifactRef     `json:"artifacts,omitempty"`
	Metadata   map[string]string `json:"metadata,omitempty"`
}

// StepResult summarizes one adapter execution and its normalized output.
type StepResult struct {
	AdapterID     string        `json:"adapter_id"`
	StepID        string        `json:"step_id"`
	GateStatus    GateStatus    `json:"gate_status"`
	StartedAt     time.Time     `json:"started_at,omitempty"`
	FinishedAt    time.Time     `json:"finished_at,omitempty"`
	Duration      time.Duration `json:"duration,omitempty"`
	ExitCode      int           `json:"exit_code,omitempty"`
	TimedOut      bool          `json:"timed_out,omitempty"`
	FailureReason string        `json:"failure_reason,omitempty"`
	Results       []Result      `json:"results,omitempty"`
	Artifacts     []ArtifactRef `json:"artifacts,omitempty"`
}

// Validate checks the required fields and enum values for a Result.
func (r Result) Validate() error {
	var errs []string
	if strings.TrimSpace(r.AdapterID) == "" {
		errs = append(errs, "adapter_id is required")
	}
	if strings.TrimSpace(r.StepID) == "" {
		errs = append(errs, "step_id is required")
	}
	if strings.TrimSpace(r.Message) == "" {
		errs = append(errs, "message is required")
	}
	if !r.Kind.Valid() {
		errs = append(errs, fmt.Sprintf("invalid kind %q", r.Kind))
	}
	if !r.Category.Valid() {
		errs = append(errs, fmt.Sprintf("invalid category %q", r.Category))
	}
	if !r.Severity.Valid() {
		errs = append(errs, fmt.Sprintf("invalid severity %q", r.Severity))
	}
	if !r.Scope.Valid() {
		errs = append(errs, fmt.Sprintf("invalid scope %q", r.Scope))
	}
	if !r.GateStatus.Valid() {
		errs = append(errs, fmt.Sprintf("invalid gate_status %q", r.GateStatus))
	}
	if !r.Fix.Safety.Valid() {
		errs = append(errs, fmt.Sprintf("invalid fix safety %q", r.Fix.Safety))
	}
	if len(errs) > 0 {
		return errors.New(strings.Join(errs, "; "))
	}
	return nil
}

// Normalize fills optional enums with explicit unknown/none defaults.
func (r Result) Normalize() Result {
	if r.Kind == "" {
		r.Kind = KindViolation
	}
	if r.Category == "" {
		r.Category = CategoryUnknown
	}
	if r.Severity == "" {
		r.Severity = SeverityUnknown
	}
	if r.Scope == "" {
		r.Scope = ScopeUnknown
	}
	if r.GateStatus == "" {
		r.GateStatus = GateFail
	}
	if r.Fix.Safety == "" {
		r.Fix.Safety = FixNone
	}
	return r
}

func (k Kind) Valid() bool {
	switch k {
	case KindViolation, KindTest, KindCoverage, KindSecurity, KindArtifact:
		return true
	default:
		return false
	}
}

func (s Severity) Valid() bool {
	switch s {
	case SeverityUnknown, SeverityInfo, SeverityLow, SeverityMedium, SeverityHigh, SeverityCritical:
		return true
	default:
		return false
	}
}

func (c Category) Valid() bool {
	switch c {
	case CategoryUnknown, CategoryFormat, CategoryLint, CategoryArchitecture, CategorySecurity, CategoryTest, CategoryCoverage, CategorySemantic, CategoryCommand:
		return true
	default:
		return false
	}
}

func (s Scope) Valid() bool {
	switch s {
	case ScopeUnknown, ScopeRepository, ScopePackage, ScopeFile, ScopeLine, ScopeSymbol:
		return true
	default:
		return false
	}
}

func (g GateStatus) Valid() bool {
	switch g {
	case GatePass, GateWarn, GateFail:
		return true
	default:
		return false
	}
}

func (f FixSafety) Valid() bool {
	switch f {
	case FixSafe, FixReview, FixNone:
		return true
	default:
		return false
	}
}

// ParseFixSafety accepts the canonical policy vocabulary plus documented aliases.
func ParseFixSafety(raw string) (FixSafety, error) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "safe":
		return FixSafe, nil
	case "review", "manual-review", "manual_review":
		return FixReview, nil
	case "", "none", "report-only", "report_only", "off", "disabled":
		return FixNone, nil
	default:
		return "", fmt.Errorf("unknown fix safety %q", raw)
	}
}

// ParseGateStatus parses pass/warn/fail gate values.
func ParseGateStatus(raw string) (GateStatus, error) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "pass", "passed", "ok", "success":
		return GatePass, nil
	case "warn", "warning":
		return GateWarn, nil
	case "fail", "failed", "error":
		return GateFail, nil
	default:
		return "", fmt.Errorf("unknown gate status %q", raw)
	}
}
