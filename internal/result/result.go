// Package result 定义 adapter、pipeline、修复策略和报告生成器共享的标准化 review 输出模型。
package result

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

// Kind 描述 adapter 输出的标准化记录类型。
type Kind string

const (
	KindViolation Kind = "violation"
	KindTest      Kind = "test"
	KindCoverage  Kind = "coverage"
	KindSecurity  Kind = "security"
	KindArtifact  Kind = "artifact"
)

// Severity 描述结果对人工处理和门禁的紧急程度。
type Severity string

const (
	SeverityUnknown  Severity = "unknown"
	SeverityInfo     Severity = "info"
	SeverityLow      Severity = "low"
	SeverityMedium   Severity = "medium"
	SeverityHigh     Severity = "high"
	SeverityCritical Severity = "critical"
)

// Category 按 review 领域给结果分组，避免核心模型绑定具体工具。
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

// Scope 标识受影响的 review 范围。
type Scope string

const (
	ScopeUnknown    Scope = "unknown"
	ScopeRepository Scope = "repository"
	ScopePackage    Scope = "package"
	ScopeFile       Scope = "file"
	ScopeLine       Scope = "line"
	ScopeSymbol     Scope = "symbol"
)

// GateStatus 是 pipeline 和报告门禁消费的状态值。
type GateStatus string

const (
	GatePass GateStatus = "pass"
	GateWarn GateStatus = "warn"
	GateFail GateStatus = "fail"
)

// FixSafety 是自动修复安全等级的标准词表。
type FixSafety string

const (
	FixSafe   FixSafety = "safe"
	FixReview FixSafety = "review"
	FixNone   FixSafety = "none"
)

// Location 在可定位时指向源码位置。
type Location struct {
	File   string `json:"file,omitempty"`
	Line   int    `json:"line,omitempty"`
	Column int    `json:"column,omitempty"`
}

// Fix 描述问题是否可修复以及修复方式。
type Fix struct {
	Available bool      `json:"available"`
	Safety    FixSafety `json:"safety"`
	Message   string    `json:"message,omitempty"`
}

// ArtifactRef 保存原始或派生的 adapter 输出，供报告和调试使用。
type ArtifactRef struct {
	Name        string `json:"name"`
	Kind        string `json:"kind,omitempty"`
	Path        string `json:"path,omitempty"`
	ContentType string `json:"content_type,omitempty"`
	Inline      string `json:"inline,omitempty"`
}

// Result 是 adapter 产生的工具无关标准化记录。
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

// StepResult 汇总一次 adapter 执行及其标准化输出。
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

// Validate 检查 Result 的必填字段和枚举值。
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

// Normalize 为可选枚举补齐显式 unknown/none 默认值。
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

// ParseFixSafety 解析标准修复安全词表及文档化别名。
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

// ParseGateStatus 解析 pass/warn/fail 门禁值。
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
