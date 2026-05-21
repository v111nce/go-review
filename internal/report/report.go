package report

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"text/template"
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
	FixApplied   bool       `json:"fix_applied"`
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
	Message       string        `json:"message,omitempty"`
	FixAvailable  bool          `json:"fix_available"`
	FixSafety     string        `json:"fix_safety,omitempty"`
	FixApplied    bool          `json:"fix_applied"`
}

// ArtifactRef points to captured command/tool output.
type ArtifactRef struct {
	StepID string `json:"step_id"`
	Name   string `json:"name"`
	Path   string `json:"path"`
}

// Summary keeps high-level counts convenient for humans, LLMs, and machines.
type Summary struct {
	StepsTotal    int `json:"steps_total"`
	StepsPassed   int `json:"steps_passed"`
	StepsFailed   int `json:"steps_failed"`
	StepsWarned   int `json:"steps_warned"`
	FindingsTotal int `json:"findings_total"`
	FixesApplied  int `json:"fixes_applied"`
}

// RunReport is the portable artifact written by JSON/Markdown/terminal writers.
type RunReport struct {
	SchemaVersion string        `json:"schema_version"`
	Command       string        `json:"command,omitempty"`
	Profile       string        `json:"profile,omitempty"`
	GateStatus    GateStatus    `json:"gate_status"`
	Workdir       string        `json:"workdir,omitempty"`
	ConfigPath    string        `json:"config_path,omitempty"`
	StartedAt     time.Time     `json:"started_at,omitempty"`
	EndedAt       time.Time     `json:"ended_at,omitempty"`
	Duration      time.Duration `json:"duration,omitempty"`
	Summary       Summary       `json:"summary"`
	Steps         []Step        `json:"steps,omitempty"`
	Findings      []Finding     `json:"findings,omitempty"`
	Artifacts     []ArtifactRef `json:"artifacts,omitempty"`
	Metadata      []KeyValue    `json:"metadata,omitempty"`
}

// KeyValue keeps metadata ordering deterministic.
type KeyValue struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

// Normalize sorts report content and refreshes derived fields to keep artifacts stable.
func (r *RunReport) Normalize() {
	if r.SchemaVersion == "" {
		r.SchemaVersion = "go-review.report.v1"
	}
	if r.Duration == 0 && !r.StartedAt.IsZero() && !r.EndedAt.IsZero() {
		r.Duration = r.EndedAt.Sub(r.StartedAt)
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
	sort.SliceStable(r.Artifacts, func(i, j int) bool {
		if r.Artifacts[i].StepID != r.Artifacts[j].StepID {
			return r.Artifacts[i].StepID < r.Artifacts[j].StepID
		}
		if r.Artifacts[i].Name != r.Artifacts[j].Name {
			return r.Artifacts[i].Name < r.Artifacts[j].Name
		}
		return r.Artifacts[i].Path < r.Artifacts[j].Path
	})
	sort.SliceStable(r.Metadata, func(i, j int) bool { return r.Metadata[i].Key < r.Metadata[j].Key })
	r.Summary = summarize(*r)
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
		loc := findingLocation(finding)
		if _, err := fmt.Fprintf(w, "- %s %s %s: %s\n", finding.GateStatus, loc, emptyDash(finding.RuleID), finding.Message); err != nil {
			return err
		}
	}
	return nil
}

// WriteMarkdown writes the human-facing review report.
func WriteMarkdown(w io.Writer, r RunReport) error {
	r.Normalize()
	const tpl = `# go-review Report

## Summary

| Field | Value |
| --- | --- |
| Status | {{.GateStatus}} |
| Command | {{dash .Command}} |
| Profile | {{dash .Profile}} |
| Workdir | {{dash .Workdir}} |
| Config | {{dash .ConfigPath}} |
| Started At | {{time .StartedAt}} |
| Duration | {{duration .Duration}} |

## Result

{{resultSentence .}}

{{.Summary.StepsPassed}} steps passed, {{.Summary.StepsFailed}} steps failed, {{.Summary.StepsWarned}} steps warned.

## Failed Findings

{{if .Findings}}| Step | Rule | Location | Message | Auto Fix |
| --- | --- | --- | --- | --- |
{{- range .Findings}}
| {{md .StepID}} | {{md (dash .RuleID)}} | {{md (location .)}} | {{md .Message}} | {{md (fix .)}} |
{{- end}}
{{else}}No failed findings.{{end}}

## Steps

| Step | Adapter | Status | Message | Fix |
| --- | --- | --- | --- | --- |
{{- range .Steps}}
| {{md .ID}} | {{md (dash .AdapterID)}} | {{.Status}} | {{md (dash .Message)}} | {{md (stepFix .)}} |
{{- end}}

## Fixes Applied

{{fixesApplied .}}

## Artifacts

{{if .Artifacts}}| Step | Name | Path |
| --- | --- | --- |
{{- range .Artifacts}}
| {{md .StepID}} | {{md .Name}} | {{md .Path}} |
{{- end}}
{{else}}No artifacts were written.{{end}}

## Next Actions

{{nextActions .}}
`
	return executeTemplate(w, "markdown", tpl, r)
}

// WriteLLMMarkdown writes deterministic repair context intended to be pasted into an LLM.
func WriteLLMMarkdown(w io.Writer, r RunReport) error {
	r.Normalize()
	const tpl = `# go-review LLM Repair Context

You are helping fix a Go project based on deterministic go-review results.

## Task

Fix the failed review findings below while preserving existing behavior.

## Project Context

- Workdir: {{dash .Workdir}}
- Command: go-review {{dash .Command}}
- Profile: {{dash .Profile}}
- Config: {{dash .ConfigPath}}
- Overall status: {{.GateStatus}}

## Important Constraints

- Do not change generated artifacts under .go-review/artifacts/ or artifacts/go-review/.
- Do not silence rules unless explicitly justified.
- Prefer small, behavior-preserving fixes.
- go-review check is read-only and should not modify source files.
- If a rule has fix_safety: safe, it may be auto-fixed by go-review fix.
- If a rule has fix_safety: review, modify code carefully and explain the change.

## Failed Findings

{{if .Findings}}{{range $i, $f := .Findings}}### Finding {{inc $i}}

- Step: {{$f.StepID}}
- Adapter: {{$f.AdapterID}}
- Rule: {{dash $f.RuleID}}
- Severity: {{dash $f.Severity}}
- File: {{dash $f.File}}
- Line: {{$f.Line}}
- Column: {{$f.Column}}
- Message: {{$f.Message}}
- Suggestion: {{dash $f.Suggestion}}
- Auto fix available: {{$f.FixAvailable}}
- Fix safety: {{dash $f.FixSafety}}
- Fix applied: {{$f.FixApplied}}

Recommended direction:

{{recommendation $f}}

{{end}}{{else}}No failed findings were reported. If the status is pass, no repair is needed.{{end}}

## Relevant Command Output

{{if .Artifacts}}Artifact paths:
{{range .Artifacts}}
- {{.StepID}} / {{.Name}}: {{.Path}}{{end}}
{{else}}No external artifact paths were reported.{{end}}

## Expected Completion Criteria

After fixing, this command should pass:

    go-review {{commandOrCheck .Command}} --profile {{dash .Profile}}

If only semantic validation is needed, run the relevant semantic profile from the project config.

## Repair Priority

1. Fix findings with exact file locations first.
2. Apply only safe automatic fixes with go-review fix when appropriate.
3. Re-run go-review check --profile {{dash .Profile}}.
4. Keep generated reports and artifacts out of source edits.
`
	return executeTemplate(w, "llm-markdown", tpl, r)
}

// WriteFiles writes latest and timestamped human, LLM, and JSON reports.
func WriteFiles(dir string, r RunReport) error {
	if strings.TrimSpace(dir) == "" {
		return nil
	}
	r.Normalize()
	if err := os.MkdirAll(filepath.Join(dir, "runs"), 0o755); err != nil {
		return err
	}
	stamp := reportStamp(r)
	files := []struct {
		path  string
		write func(io.Writer, RunReport) error
	}{
		{filepath.Join(dir, "latest.md"), WriteMarkdown},
		{filepath.Join(dir, "latest.llm.md"), WriteLLMMarkdown},
		{filepath.Join(dir, "latest.json"), WriteJSON},
		{filepath.Join(dir, "runs", stamp+".md"), WriteMarkdown},
		{filepath.Join(dir, "runs", stamp+".llm.md"), WriteLLMMarkdown},
		{filepath.Join(dir, "runs", stamp+".json"), WriteJSON},
	}
	for _, file := range files {
		if err := writeOne(file.path, r, file.write); err != nil {
			return err
		}
	}
	return nil
}

func writeOne(path string, r RunReport, write func(io.Writer, RunReport) error) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return write(f, r)
}

func executeTemplate(w io.Writer, name, tpl string, r RunReport) error {
	t, err := template.New(name).Funcs(template.FuncMap{
		"commandOrCheck": commandOrCheck,
		"dash":           emptyDash,
		"duration":       formatDuration,
		"fix":            markdownFix,
		"fixesApplied":   fixesApplied,
		"inc":            func(i int) int { return i + 1 },
		"location":       findingLocation,
		"md":             escapeMarkdownCell,
		"nextActions":    nextActions,
		"recommendation": recommendation,
		"resultSentence": resultSentence,
		"stepFix":        stepFix,
		"time":           formatTime,
	}).Parse(tpl)
	if err != nil {
		return err
	}
	return t.Execute(w, r)
}

func summarize(r RunReport) Summary {
	s := Summary{StepsTotal: len(r.Steps), FindingsTotal: len(r.Findings)}
	for _, step := range r.Steps {
		switch step.Status {
		case GatePass:
			s.StepsPassed++
		case GateWarn:
			s.StepsWarned++
		case GateFail:
			s.StepsFailed++
		}
		if step.FixApplied {
			s.FixesApplied++
		}
	}
	for _, finding := range r.Findings {
		if finding.FixApplied {
			s.FixesApplied++
		}
	}
	return s
}

func emptyDash(value string) string {
	if strings.TrimSpace(value) == "" {
		return "-"
	}
	return value
}

func findingLocation(f Finding) string {
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
	if f.FixApplied {
		return "applied"
	}
	if !f.FixAvailable {
		return "no"
	}
	if f.FixSafety == "" {
		return "available"
	}
	return f.FixSafety
}

func stepFix(s Step) string {
	if s.FixApplied {
		return "applied"
	}
	if !s.FixAvailable {
		return "-"
	}
	if s.FixSafety == "" {
		return "available"
	}
	return s.FixSafety
}

func resultSentence(r RunReport) string {
	switch r.GateStatus {
	case GatePass:
		return "✅ Review passed."
	case GateWarn:
		return "⚠️ Review completed with warnings."
	case GateFail:
		return "❌ Review failed."
	default:
		return fmt.Sprintf("Review completed with status `%s`.", r.GateStatus)
	}
}

func fixesApplied(r RunReport) string {
	if r.Summary.FixesApplied == 0 {
		if r.Command == "check" {
			return "No fixes were applied. `check` is read-only. Run `go-review fix --profile " + emptyDash(r.Profile) + "` to apply safe fixes."
		}
		return "No fixes were applied."
	}
	var lines []string
	for _, step := range r.Steps {
		if step.FixApplied {
			lines = append(lines, fmt.Sprintf("- `%s` applied safe fix `%s`.", step.ID, stepFix(step)))
		}
	}
	if len(lines) == 0 {
		return fmt.Sprintf("%d fixes were applied.", r.Summary.FixesApplied)
	}
	return strings.Join(lines, "\n")
}

func nextActions(r RunReport) string {
	if r.GateStatus == GatePass {
		return "No action required. Keep this report as CI/local verification evidence."
	}
	var lines []string
	for _, finding := range r.Findings {
		loc := findingLocation(finding)
		if loc == "-" {
			lines = append(lines, fmt.Sprintf("- Fix `%s`: %s", emptyDash(finding.RuleID), finding.Message))
			continue
		}
		lines = append(lines, fmt.Sprintf("- Fix `%s` at `%s`: %s", emptyDash(finding.RuleID), loc, finding.Message))
	}
	if len(lines) == 0 {
		lines = append(lines, "- Inspect failed steps and artifact output listed above.")
	}
	lines = append(lines, fmt.Sprintf("- Re-run `go-review check --profile %s`.", emptyDash(r.Profile)))
	return strings.Join(lines, "\n")
}

func recommendation(f Finding) string {
	if strings.TrimSpace(f.Suggestion) != "" {
		return f.Suggestion
	}
	if f.FixAvailable && f.FixSafety == "safe" {
		return "This finding is marked safe to auto-fix. Prefer running `go-review fix` before manual edits."
	}
	return "Use the rule, message, location, and artifacts above to make the smallest behavior-preserving change."
}

func formatTime(t time.Time) string {
	if t.IsZero() {
		return "-"
	}
	return t.UTC().Format(time.RFC3339)
}

func formatDuration(d time.Duration) string {
	if d <= 0 {
		return "-"
	}
	return d.Round(time.Millisecond).String()
}

func commandOrCheck(command string) string {
	command = strings.TrimSpace(command)
	if command == "" || command == "-" {
		return "check"
	}
	return command
}

func escapeMarkdownCell(value string) string {
	value = strings.ReplaceAll(value, "|", "\\|")
	value = strings.ReplaceAll(value, "\n", "<br>")
	value = strings.TrimSpace(value)
	if value == "" {
		return "-"
	}
	return value
}

func reportStamp(r RunReport) string {
	t := r.StartedAt
	if t.IsZero() {
		t = time.Now().UTC()
	}
	return t.UTC().Format("20060102T150405Z")
}
