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

// GateStatus 复用稳定序列化状态契约，同时避免直接导入 pipeline 包。
type GateStatus string

const (
	GatePass    GateStatus = "pass"
	GateWarn    GateStatus = "warn"
	GateFail    GateStatus = "fail"
	GateSkipped GateStatus = "skipped"
)

// Finding 是适合报告展示的标准化结果子集。
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

// Step 是适合报告展示的 pipeline 执行子集。
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

// ArtifactRef 指向捕获到的命令或工具输出。
type ArtifactRef struct {
	StepID string `json:"step_id"`
	Name   string `json:"name"`
	Path   string `json:"path"`
}

// Summary 保存人、LLM 和机器都容易消费的高层计数。
type Summary struct {
	StepsTotal    int `json:"steps_total"`
	StepsPassed   int `json:"steps_passed"`
	StepsFailed   int `json:"steps_failed"`
	StepsWarned   int `json:"steps_warned"`
	FindingsTotal int `json:"findings_total"`
	FixesApplied  int `json:"fixes_applied"`
}

// RunReport 是 JSON、Markdown 和终端 writer 共用的可移植报告产物。
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

// KeyValue 让 metadata 保持稳定排序。
type KeyValue struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

// Normalize 对报告内容排序并刷新派生字段，保证产物稳定。
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

// WriteJSON 写入稳定的机器可读报告。
func WriteJSON(w io.Writer, r RunReport) error {
	r.Normalize()
	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	return encoder.Encode(r)
}

// WriteTerminal 写入适合本地门禁查看的紧凑人类可读摘要。
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

// WriteMarkdown 写入面向人的 review 报告。
func WriteMarkdown(w io.Writer, r RunReport) error {
	r.Normalize()
	const tpl = `# go-review 报告

## 摘要

| 字段 | 值 |
| --- | --- |
| 状态 | {{.GateStatus}} |
| 命令 | {{dash .Command}} |
| Profile | {{dash .Profile}} |
| 工作目录 | {{dash .Workdir}} |
| 配置 | {{dash .ConfigPath}} |
| 开始时间 | {{time .StartedAt}} |
| 耗时 | {{duration .Duration}} |

## 结果

{{resultSentence .}}

{{.Summary.StepsPassed}} 个步骤通过，{{.Summary.StepsFailed}} 个步骤失败，{{.Summary.StepsWarned}} 个步骤告警。

## 失败项

{{if .Findings}}| 步骤 | 规则 | 位置 | 信息 | 自动修复 |
| --- | --- | --- | --- | --- |
{{- range .Findings}}
| {{md .StepID}} | {{md (dash .RuleID)}} | {{md (location .)}} | {{md .Message}} | {{md (fix .)}} |
{{- end}}
{{else}}没有失败项。{{end}}

## 步骤

| 步骤 | Adapter | 状态 | 信息 | 修复 |
| --- | --- | --- | --- | --- |
{{- range .Steps}}
| {{md .ID}} | {{md (dash .AdapterID)}} | {{.Status}} | {{md (dash .Message)}} | {{md (stepFix .)}} |
{{- end}}

## 已应用修复

{{fixesApplied .}}

## 产物

{{if .Artifacts}}| 步骤 | 名称 | 路径 |
| --- | --- | --- |
{{- range .Artifacts}}
| {{md .StepID}} | {{md .Name}} | {{md .Path}} |
{{- end}}
{{else}}没有写入产物。{{end}}

## 下一步

{{nextActions .}}
`
	return executeTemplate(w, "markdown", tpl, r)
}

// WriteLLMMarkdown 写入可复制给 LLM 的确定性修复上下文。
func WriteLLMMarkdown(w io.Writer, r RunReport) error {
	r.Normalize()
	const tpl = `# go-review LLM 修复上下文

你正在根据确定性的 go-review 结果修复 Go 项目。

## 任务

在保持现有行为的前提下，修复下面的失败 review 项。

## 项目上下文

- 工作目录：{{dash .Workdir}}
- 命令：go-review {{dash .Command}}
- Profile：{{dash .Profile}}
- 配置：{{dash .ConfigPath}}
- 整体状态：{{.GateStatus}}

## 重要约束

- 不要修改 .go-review/artifacts/ 或 artifacts/go-review/ 下的生成产物。
- 除非有明确理由，不要通过关闭规则来规避问题。
- 优先做小而保持行为不变的修改。
- go-review check 是只读命令，不应修改源码。
- 如果规则的 fix_safety 是 safe，可以优先用 go-review fix 自动修复。
- 如果规则的 fix_safety 是 review，需要谨慎手改并说明原因。

## LLM 审阅规则

{{llmRulesSection .}}

## 失败项

{{if .Findings}}{{range $i, $f := .Findings}}### 失败项 {{inc $i}}

- 步骤：{{$f.StepID}}
- Adapter：{{$f.AdapterID}}
- 规则：{{dash $f.RuleID}}
- 严重级别：{{dash $f.Severity}}
- 文件：{{dash $f.File}}
- 行：{{$f.Line}}
- 列：{{$f.Column}}
- 信息：{{$f.Message}}
- 建议：{{dash $f.Suggestion}}
- 可自动修复：{{$f.FixAvailable}}
- 修复安全级别：{{dash $f.FixSafety}}
- 已应用修复：{{$f.FixApplied}}

建议方向：

{{recommendation $f}}

{{end}}{{else}}没有报告失败项。如果状态是 pass，则不需要修复。{{end}}

## 相关命令输出

{{if .Artifacts}}产物路径：
{{range .Artifacts}}
- {{.StepID}} / {{.Name}}：{{.Path}}{{end}}
{{else}}没有报告外部产物路径。{{end}}

## 预期完成标准

修复后，下面命令应通过：

    go-review {{commandOrCheck .Command}} --profile {{dash .Profile}}

如果只需要验证语义规则，请运行项目配置里对应的 semantic profile。

## 修复优先级

1. 优先修复带有明确文件位置的失败项。
2. 只对标记为 safe 的项使用 go-review fix 自动修复。
3. 重新运行 go-review check --profile {{dash .Profile}}。
4. 不要把生成报告和产物纳入源码修改。
`
	return executeTemplate(w, "llm-markdown", tpl, r)
}

// WriteProcessMarkdown 写入统一过程文档。
//
// 这个文档是 review/fix 的主阅读入口：它按执行过程解释哪些 safe fix 已应用、哪些
// lint/semantic/test 只提供检测意见、后续 LLM step 的修改/复盘输出在哪里。latest.llm.md
// 继续保留为模型输入上下文，但不承担人类主报告职责。
func WriteProcessMarkdown(w io.Writer, r RunReport) error {
	r.Normalize()
	const tpl = `# go-review 过程文档

## 0. 运行摘要

| 字段 | 值 |
| --- | --- |
| 状态 | {{.GateStatus}} |
| 命令 | {{dash .Command}} |
| Profile | {{dash .Profile}} |
| 工作目录 | {{dash .Workdir}} |
| 配置 | {{dash .ConfigPath}} |
| 开始时间 | {{time .StartedAt}} |
| 耗时 | {{duration .Duration}} |

## 1. Safe fix 执行结果

{{safeFixSection .}}

## 2. 工具检测结果：lint / semantic / test

{{toolDetectionSection .}}

说明：只检测不修复的规则不会自动改代码；它们通过本节给出 rule_id、步骤、文件位置、失败信息和建议。若 gate 为 fail，终端会显示 FAILED，并在 latest.md / latest.process.md 中保留具体失败项。

## 3. 第一模型执行结果

{{llmStepSection . "llm-review"}}

## 4. LLM 规则审阅与修复说明

LLM 规则来源：.go-review/llm/default.json 和 .go-review/llm/custom.json。default 由框架维护，custom 由用户维护；这些规则需要上下文判断，默认不作为确定性工具 gate。

{{llmRulesSection .}}

## 5. 第二模型复盘结果

{{llmStepSection . "llm-claude"}}

## 6. 产物索引

{{if .Artifacts}}| 步骤 | 名称 | 路径 |
| --- | --- | --- |
{{- range .Artifacts}}
| {{md .StepID}} | {{md .Name}} | {{md .Path}} |
{{- end}}
{{else}}没有写入产物。{{end}}
`
	return executeTemplate(w, "process-markdown", tpl, r)
}

// WriteFiles 同时写入 latest 和带时间戳的人类、LLM、JSON 报告。
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
		{filepath.Join(dir, "latest.process.md"), WriteProcessMarkdown},
		{filepath.Join(dir, "latest.json"), WriteJSON},
		{filepath.Join(dir, "runs", stamp+".md"), WriteMarkdown},
		{filepath.Join(dir, "runs", stamp+".llm.md"), WriteLLMMarkdown},
		{filepath.Join(dir, "runs", stamp+".process.md"), WriteProcessMarkdown},
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
		"commandOrCheck":       commandOrCheck,
		"dash":                 emptyDash,
		"duration":             formatDuration,
		"fix":                  markdownFix,
		"fixesApplied":         fixesApplied,
		"inc":                  func(i int) int { return i + 1 },
		"location":             findingLocation,
		"llmRulesSection":      llmRulesSection,
		"md":                   escapeMarkdownCell,
		"nextActions":          nextActions,
		"recommendation":       recommendation,
		"safeFixSection":       safeFixSection,
		"toolDetectionSection": toolDetectionSection,
		"llmStepSection":       llmStepSection,
		"resultSentence":       resultSentence,
		"stepFix":              stepFix,
		"time":                 formatTime,
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
		return "已应用"
	}
	if !f.FixAvailable {
		return "否"
	}
	if f.FixSafety == "" {
		return "可用"
	}
	return f.FixSafety
}

func stepFix(s Step) string {
	if s.FixApplied {
		return "已应用"
	}
	if !s.FixAvailable {
		return "-"
	}
	if s.FixSafety == "" {
		return "可用"
	}
	return s.FixSafety
}

func resultSentence(r RunReport) string {
	switch r.GateStatus {
	case GatePass:
		return "✅ Review 通过。"
	case GateWarn:
		return "⚠️ Review 完成，但存在告警。"
	case GateFail:
		return "❌ Review 失败。"
	default:
		return fmt.Sprintf("Review 完成，状态为 `%s`。", r.GateStatus)
	}
}

func fixesApplied(r RunReport) string {
	if r.Summary.FixesApplied == 0 {
		if r.Command == "check" {
			return "没有应用修复。`check` 是只读命令；如需应用安全修复，请运行 `go-review fix --profile " + emptyDash(r.Profile) + "`。"
		}
		return "没有应用修复。"
	}
	var lines []string
	for _, step := range r.Steps {
		if step.FixApplied {
			lines = append(lines, fmt.Sprintf("- `%s` 已应用安全修复 `%s`。", step.ID, stepFix(step)))
		}
	}
	if len(lines) == 0 {
		return fmt.Sprintf("已应用 %d 个修复。", r.Summary.FixesApplied)
	}
	return strings.Join(lines, "\n")
}

func nextActions(r RunReport) string {
	if r.GateStatus == GatePass {
		return "无需操作。请保留此报告作为 CI/本地验证证据。"
	}
	var lines []string
	for _, finding := range r.Findings {
		loc := findingLocation(finding)
		if loc == "-" {
			lines = append(lines, fmt.Sprintf("- 修复 `%s`：%s", emptyDash(finding.RuleID), finding.Message))
			continue
		}
		lines = append(lines, fmt.Sprintf("- 修复 `%s`，位置 `%s`：%s", emptyDash(finding.RuleID), loc, finding.Message))
	}
	if len(lines) == 0 {
		lines = append(lines, "- 检查上方失败步骤和产物输出。")
	}
	lines = append(lines, fmt.Sprintf("- 重新运行 `go-review check --profile %s`。", emptyDash(r.Profile)))
	return strings.Join(lines, "\n")
}

func recommendation(f Finding) string {
	if strings.TrimSpace(f.Suggestion) != "" {
		return f.Suggestion
	}
	if f.FixAvailable && f.FixSafety == "safe" {
		return "该失败项标记为可安全自动修复。手动修改前，优先运行 `go-review fix`。"
	}
	return "根据上方规则、信息、位置和产物，做最小且保持行为不变的修改。"
}

func llmRulesSection(r RunReport) string {
	paths := llmRulesPaths(r.ConfigPath)
	if len(paths) == 0 {
		return "- 未发现配置路径；如需 LLM 审阅，请在项目内维护 .go-review/llm/default.json 和 .go-review/llm/custom.json。"
	}
	var b strings.Builder
	fmt.Fprintln(&b, "- 规则文件：")
	for _, source := range paths {
		fmt.Fprintf(&b, "  - %s：`%s`\n", source.Name, source.Path)
		if data, err := os.ReadFile(source.Path); err == nil {
			summary := summarizeLLMRules(data, source.Name)
			if summary != "" {
				b.WriteString(summary)
			} else {
				fmt.Fprintf(&b, "    - %s 规则文件存在，但未能生成摘要；请直接读取该 JSON。\n", source.Name)
			}
		} else {
			fmt.Fprintf(&b, "    - 当前未能读取 %s 规则文件：%v\n", source.Name, err)
		}
	}
	b.WriteString("- 审阅输出必须带 `rule_id`、文件位置、原因和建议；C 类规则默认不作为确定性硬 gate。\n")
	return strings.TrimRight(b.String(), "\n")
}

type llmRuleSource struct {
	Name string
	Path string
}

func llmRulesPaths(configPath string) []llmRuleSource {
	configPath = strings.TrimSpace(configPath)
	if configPath == "" || configPath == "-" {
		return nil
	}
	dir := filepath.Dir(configPath)
	return []llmRuleSource{
		{Name: "default", Path: filepath.Join(dir, "llm", "default.json")},
		{Name: "custom", Path: filepath.Join(dir, "llm", "custom.json")},
	}
}

func summarizeLLMRules(data []byte, sourceName string) string {
	var raw struct {
		Rules []struct {
			ID          string `json:"id"`
			Title       string `json:"title"`
			Description string `json:"description"`
			Handling    string `json:"handling"`
		} `json:"rules"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return ""
	}
	total := 0
	var lines []string
	for _, rule := range raw.Rules {
		if rule.Handling != "llm-review" {
			continue
		}
		total++
		if len(lines) < 20 {
			title := strings.TrimSpace(rule.Title)
			if title == "" {
				title = strings.TrimSpace(rule.Description)
			}
			lines = append(lines, fmt.Sprintf("    - `%s`：%s", rule.ID, emptyDash(title)))
		}
	}
	var b strings.Builder
	fmt.Fprintf(&b, "    - %s LLM 规则数量：%d\n", sourceName, total)
	if len(lines) > 0 {
		fmt.Fprintf(&b, "    - %s 规则摘要（前 20 条）：\n", sourceName)
		b.WriteString(strings.Join(lines, "\n"))
		b.WriteString("\n")
	}
	if total > len(lines) {
		fmt.Fprintf(&b, "    - %s 其余 %d 条请读取规则文件。\n", sourceName, total-len(lines))
	}
	return b.String()
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

func safeFixSection(r RunReport) string {
	var rows []string
	for _, step := range r.Steps {
		if step.FixSafety == "safe" || step.FixAvailable || step.FixApplied {
			rows = append(rows, fmt.Sprintf("| %s | %s | %s | %s | %s |", escapeMarkdownCell(step.ID), escapeMarkdownCell(dash(step.AdapterID)), escapeMarkdownCell(string(step.Status)), escapeMarkdownCell(stepFix(step)), escapeMarkdownCell(dash(step.Message))))
		}
	}
	if len(rows) == 0 {
		return "本次没有 safe fix 步骤或没有应用 safe fix。"
	}
	return "| 步骤 | Adapter | 状态 | 修复 | 信息 |\n| --- | --- | --- | --- | --- |\n" + strings.Join(rows, "\n")
}

func toolDetectionSection(r RunReport) string {
	var b strings.Builder
	var stepRows []string
	for _, step := range r.Steps {
		if isLLMStep(step) {
			continue
		}
		stepRows = append(stepRows, fmt.Sprintf("| %s | %s | %s | %s | %s |", escapeMarkdownCell(step.ID), escapeMarkdownCell(dash(step.AdapterID)), escapeMarkdownCell(string(step.Status)), escapeMarkdownCell(stepFix(step)), escapeMarkdownCell(dash(step.Message))))
	}
	if len(stepRows) == 0 {
		b.WriteString("没有工具检测步骤。\n")
	} else {
		b.WriteString("### 步骤结果\n\n| 步骤 | Adapter | 状态 | 修复 | 信息 |\n| --- | --- | --- | --- | --- |\n")
		b.WriteString(strings.Join(stepRows, "\n"))
		b.WriteString("\n")
	}
	var findingRows []string
	for _, finding := range r.Findings {
		if strings.HasPrefix(finding.AdapterID, "llm.") || strings.HasPrefix(finding.StepID, "llm-") {
			continue
		}
		findingRows = append(findingRows, fmt.Sprintf("| %s | %s | %s | %s | %s |", escapeMarkdownCell(finding.StepID), escapeMarkdownCell(dash(finding.RuleID)), escapeMarkdownCell(findingLocation(finding)), escapeMarkdownCell(finding.Message), escapeMarkdownCell(dash(finding.Suggestion))))
	}
	if len(findingRows) == 0 {
		b.WriteString("\n### 失败/意见\n\n没有工具检测失败项。")
	} else {
		b.WriteString("\n### 失败/意见\n\n| 步骤 | 规则 | 位置 | 信息 | 建议 |\n| --- | --- | --- | --- | --- |\n")
		b.WriteString(strings.Join(findingRows, "\n"))
	}
	return b.String()
}

func llmStepSection(r RunReport, stepID string) string {
	step, ok := findProcessStep(r, stepID)
	if !ok {
		return fmt.Sprintf("未配置或未运行 `%s`。", stepID)
	}
	var b strings.Builder
	fmt.Fprintf(&b, "- 步骤：`%s`\n", step.ID)
	fmt.Fprintf(&b, "- Adapter：`%s`\n", dash(step.AdapterID))
	fmt.Fprintf(&b, "- 状态：`%s`\n", step.Status)
	fmt.Fprintf(&b, "- 信息：%s\n", dash(step.Message))
	if step.FixApplied {
		fmt.Fprintf(&b, "- 修复：已应用\n")
	}
	paths := artifactPathsForStep(r, stepID)
	if len(paths) == 0 {
		b.WriteString("- 产物：无\n")
	} else {
		b.WriteString("- 产物：\n")
		for _, artifact := range paths {
			fmt.Fprintf(&b, "  - `%s`: %s\n", artifact.Name, artifact.Path)
		}
	}
	return b.String()
}

func findProcessStep(r RunReport, stepID string) (Step, bool) {
	for _, step := range r.Steps {
		if step.ID == stepID {
			return step, true
		}
	}
	return Step{}, false
}

func artifactPathsForStep(r RunReport, stepID string) []ArtifactRef {
	var out []ArtifactRef
	for _, artifact := range r.Artifacts {
		if artifact.StepID == stepID {
			out = append(out, artifact)
		}
	}
	return out
}

func isLLMStep(step Step) bool {
	return strings.HasPrefix(step.AdapterID, "llm.") || strings.HasPrefix(step.ID, "llm-")
}

func dash(value string) string {
	return emptyDash(value)
}
