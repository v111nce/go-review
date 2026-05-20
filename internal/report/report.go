package report

import (
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"sort"
	"strings"
	"time"
)

// GateStatus mirrors the stable serialized status contract without importing pipeline.
type GateStatus string

const (
	GatePass    GateStatus = "pass"
	GateWarn    GateStatus = "warn"
	GateFail    GateStatus = "fail"
	GateSkipped GateStatus = "skipped"
)

// Finding is the reportable normalized result subset.
type Finding struct {
	AdapterID    string     `json:"adapter_id"`
	StepID       string     `json:"step_id"`
	RuleID       string     `json:"rule_id,omitempty"`
	Kind         string     `json:"kind,omitempty"`
	Category     string     `json:"category,omitempty"`
	Severity     string     `json:"severity,omitempty"`
	Scope        string     `json:"scope,omitempty"`
	File         string     `json:"file,omitempty"`
	Line         int        `json:"line,omitempty"`
	Column       int        `json:"column,omitempty"`
	Message      string     `json:"message"`
	Suggestion   string     `json:"suggestion,omitempty"`
	FixAvailable bool       `json:"fix_available"`
	FixSafety    string     `json:"fix_safety,omitempty"`
	GateStatus   GateStatus `json:"gate_status"`
}

// Step is the reportable pipeline execution subset.
type Step struct {
	ID            string        `json:"id"`
	AdapterID     string        `json:"adapter_id,omitempty"`
	Status        GateStatus    `json:"gate_status"`
	Duration      time.Duration `json:"duration,omitempty"`
	ArtifactPaths []string      `json:"artifact_paths,omitempty"`
	FailureReason string        `json:"failure_reason,omitempty"`
}

// RunReport is the portable artifact written by JSON/Markdown/terminal writers.
type RunReport struct {
	SchemaVersion string     `json:"schema_version"`
	Profile       string     `json:"profile,omitempty"`
	GateStatus    GateStatus `json:"gate_status"`
	StartedAt     time.Time  `json:"started_at,omitempty"`
	EndedAt       time.Time  `json:"ended_at,omitempty"`
	Steps         []Step     `json:"steps,omitempty"`
	Findings      []Finding  `json:"findings,omitempty"`
	Artifacts     []string   `json:"artifacts,omitempty"`
	Metadata      []KeyValue `json:"metadata,omitempty"`
}

// KeyValue keeps metadata ordering deterministic.
type KeyValue struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

// Normalize sorts report content to keep artifacts stable across scheduler implementations.
func (r *RunReport) Normalize() {
	if r.SchemaVersion == "" {
		r.SchemaVersion = "go-review.report.v1"
	}
	sort.SliceStable(r.Steps, func(i, j int) bool { return r.Steps[i].ID < r.Steps[j].ID })
	sort.SliceStable(r.Findings, func(i, j int) bool {
		a, b := r.Findings[i], r.Findings[j]
		if a.StepID != b.StepID {
			return a.StepID < b.StepID
		}
		if a.File != b.File {
			return a.File < b.File
		}
		if a.Line != b.Line {
			return a.Line < b.Line
		}
		return a.RuleID < b.RuleID
	})
	sort.Strings(r.Artifacts)
	sort.SliceStable(r.Metadata, func(i, j int) bool { return r.Metadata[i].Key < r.Metadata[j].Key })
}

// WriteJSON writes a stable machine-readable report.
func WriteJSON(w io.Writer, r RunReport) error {
	r.Normalize()
	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	return encoder.Encode(r)
}

// WriteTerminal writes a compact human-readable summary suitable for local gates.
func WriteTerminal(w io.Writer, r RunReport) error {
	r.Normalize()
	if _, err := fmt.Fprintf(w, "go-review profile=%s gate=%s findings=%d\n", emptyDash(r.Profile), r.GateStatus, len(r.Findings)); err != nil {
		return err
	}
	for _, step := range r.Steps {
		if _, err := fmt.Fprintf(w, "[%s] %s", step.Status, step.ID); err != nil {
			return err
		}
		if step.AdapterID != "" {
			if _, err := fmt.Fprintf(w, " adapter=%s", step.AdapterID); err != nil {
				return err
			}
		}
		if step.FailureReason != "" {
			if _, err := fmt.Fprintf(w, " reason=%s", step.FailureReason); err != nil {
				return err
			}
		}
		if _, err := fmt.Fprintln(w); err != nil {
			return err
		}
	}
	for _, finding := range r.Findings {
		loc := finding.File
		if finding.Line > 0 {
			loc = fmt.Sprintf("%s:%d", loc, finding.Line)
			if finding.Column > 0 {
				loc = fmt.Sprintf("%s:%d", loc, finding.Column)
			}
		}
		if loc == "" {
			loc = "-"
		}
		if _, err := fmt.Fprintf(w, "- %s %s %s: %s\n", finding.GateStatus, loc, emptyDash(finding.RuleID), finding.Message); err != nil {
			return err
		}
	}
	return nil
}

// WriteMarkdown writes a portable CI/nightly artifact.
func WriteMarkdown(w io.Writer, r RunReport) error {
	r.Normalize()
	const tpl = `# go-review report

- Profile: {{dash .Profile}}
- Gate: {{.GateStatus}}
- Findings: {{len .Findings}}

## Steps

| Step | Adapter | Status | Reason |
| --- | --- | --- | --- |
{{- range .Steps }}
| {{.ID}} | {{dash .AdapterID}} | {{.Status}} | {{dash .FailureReason}} |
{{- end }}

## Findings

| Status | Location | Rule | Message | Fix |
| --- | --- | --- | --- | --- |
{{- range .Findings }}
| {{.GateStatus}} | {{location .}} | {{dash .RuleID}} | {{.Message}} | {{fix .}} |
{{- end }}
`
	t, err := template.New("markdown").Funcs(template.FuncMap{
		"dash":     emptyDash,
		"location": markdownLocation,
		"fix":      markdownFix,
	}).Parse(tpl)
	if err != nil {
		return err
	}
	return t.Execute(w, r)
}

func emptyDash(value string) string {
	if strings.TrimSpace(value) == "" {
		return "-"
	}
	return value
}

func markdownLocation(f Finding) string {
	if f.File == "" {
		return "-"
	}
	if f.Line == 0 {
		return f.File
	}
	if f.Column == 0 {
		return fmt.Sprintf("%s:%d", f.File, f.Line)
	}
	return fmt.Sprintf("%s:%d:%d", f.File, f.Line, f.Column)
}

func markdownFix(f Finding) string {
	if !f.FixAvailable {
		return "-"
	}
	if f.FixSafety == "" {
		return "available"
	}
	return f.FixSafety
}
