package rulecatalog

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
)

// SchemaVersion 是规则 catalog 的机器可读契约版本。
//
// 只要 JSON 字段语义发生不兼容变化，就需要升级该版本，避免旧 CLI 误读新规则数据。
const SchemaVersion = "go-review.rules.v1"

// Catalog 是 rules/go-rules.json 的根对象。
// 它是规则治理的唯一机器可读源，Markdown 文档只应由它渲染生成。
type Catalog struct {
	SchemaVersion string `json:"schema_version"`
	Rules         []Rule `json:"rules"`
}

// Rule 描述一条可治理的 Go 规范规则。
//
// 字段设计重点是把“规范来源”“如何处理”“是否已实现”“报告 rule_id”绑定起来，
// 让检查结果可以从报告一路追溯回规则 catalog。
type Rule struct {
	ID             string   `json:"id"`
	Title          string   `json:"title,omitempty"`
	Description    string   `json:"description"`
	Source         Source   `json:"source"`
	Handling       string   `json:"handling"`
	Adapter        string   `json:"adapter,omitempty"`
	ToolRules      []string `json:"tool_rules,omitempty"`
	DefaultProfile string   `json:"default_profile,omitempty"`
	Severity       string   `json:"severity,omitempty"`
	Autofix        Autofix  `json:"autofix"`
	Status         string   `json:"status"`
	Implemented    bool     `json:"implemented"`
	Notes          string   `json:"notes,omitempty"`
	LastVerifiedAt string   `json:"last_verified_at,omitempty"`
	Tags           []string `json:"tags,omitempty"`
}

// Source 记录规则来源，便于后续定期回看官方/社区规范是否更新。
type Source struct {
	Name    string `json:"name"`
	URL     string `json:"url,omitempty"`
	Section string `json:"section,omitempty"`
}

// Autofix 描述规则是否支持自动修复以及修复安全等级。
// Supported=false 时 safety 应为 none，避免报告误导用户该规则可以自动改。
type Autofix struct {
	Supported bool   `json:"supported"`
	Safety    string `json:"safety,omitempty"`
}

// Empty 返回一个带 schema_version 的空 catalog，用于 `rules add/upsert` 创建新文件。
func Empty() Catalog {
	return Catalog{SchemaVersion: SchemaVersion, Rules: []Rule{}}
}

// LoadFile 从磁盘读取并校验 catalog。
func LoadFile(path string) (Catalog, error) {
	f, err := os.Open(path)
	if err != nil {
		return Catalog{}, err
	}
	defer f.Close()
	return Load(f)
}

// Load 读取 catalog JSON，并拒绝未知字段。
//
// DisallowUnknownFields 可以防止维护 catalog 时拼错字段名却静默通过，例如把
// implemented 写成 implementd 导致规则状态失真。
func Load(r io.Reader) (Catalog, error) {
	var catalog Catalog
	decoder := json.NewDecoder(r)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&catalog); err != nil {
		return Catalog{}, err
	}
	if catalog.SchemaVersion == "" {
		catalog.SchemaVersion = SchemaVersion
	}
	catalog.Normalize()
	if err := catalog.Validate(); err != nil {
		return Catalog{}, err
	}
	return catalog, nil
}

// SaveFile 规范化、校验并用稳定缩进写回 catalog。
func SaveFile(path string, catalog Catalog) error {
	catalog.Normalize()
	if err := catalog.Validate(); err != nil {
		return err
	}
	var buf bytes.Buffer
	encoder := json.NewEncoder(&buf)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(catalog); err != nil {
		return err
	}
	return os.WriteFile(path, buf.Bytes(), 0o644)
}

// Normalize 清理 catalog 中的空白、默认值和排序。
// 稳定排序能让 CRUD 造成的 diff 可读，避免规则顺序因输入文件不同而抖动。
func (c *Catalog) Normalize() {
	if c.SchemaVersion == "" {
		c.SchemaVersion = SchemaVersion
	}
	for i := range c.Rules {
		c.Rules[i].Normalize()
	}
	sort.SliceStable(c.Rules, func(i, j int) bool { return c.Rules[i].ID < c.Rules[j].ID })
}

// Normalize 清理单条规则，并补齐最小默认值。
func (r *Rule) Normalize() {
	r.ID = strings.TrimSpace(r.ID)
	r.Title = strings.TrimSpace(r.Title)
	r.Description = strings.TrimSpace(r.Description)
	r.Source.Name = strings.TrimSpace(r.Source.Name)
	r.Source.URL = strings.TrimSpace(r.Source.URL)
	r.Source.Section = strings.TrimSpace(r.Source.Section)
	r.Handling = strings.TrimSpace(r.Handling)
	r.Adapter = strings.TrimSpace(r.Adapter)
	r.DefaultProfile = strings.TrimSpace(r.DefaultProfile)
	r.Severity = strings.TrimSpace(r.Severity)
	r.Status = strings.TrimSpace(r.Status)
	r.Notes = strings.TrimSpace(r.Notes)
	r.LastVerifiedAt = strings.TrimSpace(r.LastVerifiedAt)
	if r.Status == "" {
		r.Status = "active"
	}
	if r.Autofix.Safety == "" && !r.Autofix.Supported {
		r.Autofix.Safety = "none"
	}
	r.ToolRules = normalizeStrings(r.ToolRules)
	r.Tags = normalizeStrings(r.Tags)
}

// Validate 校验整个 catalog，包括 schema、单条规则和 rule_id 唯一性。
func (c Catalog) Validate() error {
	var errs []string
	if c.SchemaVersion != SchemaVersion {
		errs = append(errs, fmt.Sprintf("unsupported schema_version %q", c.SchemaVersion))
	}
	seen := map[string]struct{}{}
	for i, rule := range c.Rules {
		if err := rule.Validate(); err != nil {
			errs = append(errs, fmt.Sprintf("rules[%d] %s: %v", i, rule.ID, err))
		}
		if rule.ID != "" {
			if _, ok := seen[rule.ID]; ok {
				errs = append(errs, fmt.Sprintf("duplicate rule id %q", rule.ID))
			}
			seen[rule.ID] = struct{}{}
		}
	}
	if len(errs) > 0 {
		return errors.New(strings.Join(errs, "; "))
	}
	return nil
}

// Validate 校验单条规则字段。
// 这里刻意拒绝 mixed 等模糊处理方式，保证 catalog 只有确定工具处理、LLM review
// 或 candidate 三类清晰路径。
func (r Rule) Validate() error {
	var errs []string
	if r.ID == "" {
		errs = append(errs, "id is required")
	}
	if r.Description == "" {
		errs = append(errs, "description is required")
	}
	if r.Source.Name == "" {
		errs = append(errs, "source.name is required")
	}
	if !validHandling(r.Handling) {
		errs = append(errs, fmt.Sprintf("invalid handling %q", r.Handling))
	}
	if !validStatus(r.Status) {
		errs = append(errs, fmt.Sprintf("invalid status %q", r.Status))
	}
	if !validAutofixSafety(r.Autofix.Safety) {
		errs = append(errs, fmt.Sprintf("invalid autofix.safety %q", r.Autofix.Safety))
	}
	if r.Autofix.Supported && r.Autofix.Safety == "none" {
		errs = append(errs, "autofix.safety cannot be none when autofix.supported is true")
	}
	if len(errs) > 0 {
		return errors.New(strings.Join(errs, "; "))
	}
	return nil
}

// Get 按稳定 rule_id 查找规则。
func (c Catalog) Get(id string) (Rule, bool) {
	id = strings.TrimSpace(id)
	for _, rule := range c.Rules {
		if rule.ID == id {
			return rule, true
		}
	}
	return Rule{}, false
}

// Add 新增规则；如果 rule_id 已存在则失败，避免意外覆盖。
func (c *Catalog) Add(rule Rule) error {
	rule.Normalize()
	if err := rule.Validate(); err != nil {
		return err
	}
	if _, ok := c.Get(rule.ID); ok {
		return fmt.Errorf("rule %q already exists", rule.ID)
	}
	c.Rules = append(c.Rules, rule)
	c.Normalize()
	return nil
}

// Upsert 新增或替换规则，用于批量同步 catalog 数据。
func (c *Catalog) Upsert(rule Rule) error {
	rule.Normalize()
	if err := rule.Validate(); err != nil {
		return err
	}
	for i := range c.Rules {
		if c.Rules[i].ID == rule.ID {
			c.Rules[i] = rule
			c.Normalize()
			return nil
		}
	}
	c.Rules = append(c.Rules, rule)
	c.Normalize()
	return nil
}

// Delete 删除指定 rule_id。
func (c *Catalog) Delete(id string) error {
	id = strings.TrimSpace(id)
	for i := range c.Rules {
		if c.Rules[i].ID == id {
			c.Rules = append(c.Rules[:i], c.Rules[i+1:]...)
			return nil
		}
	}
	return fmt.Errorf("rule %q not found", id)
}

// RenderMarkdown 把机器可读 catalog 渲染成人类可读质量文档。
//
// Markdown 不是源数据；如果需要改规则，应先通过 CRUD 修改 JSON，再重新 render-doc。
func RenderMarkdown(w io.Writer, catalog Catalog) error {
	catalog.Normalize()
	if _, err := fmt.Fprintln(w, "# Go 规则 Catalog"); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(w, "\n> 本文档由 JSON catalog 生成；请通过 `go-review rules` CRUD 命令维护源数据。"); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(w, "\n本 catalog 用来把 Go 官方、Google Go Style Guide、Uber Go Style Guide 中可复用的规范沉淀成 `go-review` 可治理的规则目录。它不是说所有规则都已经实现；它先记录来源、规则说明、处理方式、推荐承接和默认启用策略，后续再按优先级接入 `golangci-lint`、`go test`、`govulncheck`、`gosec` 或 `go.semantic`。"); err != nil {
		return err
	}
	if err := renderHandlingDefinitions(w); err != nil {
		return err
	}
	if err := renderDefaultProfiles(w); err != nil {
		return err
	}
	if err := renderJSONContract(w); err != nil {
		return err
	}
	return renderGroupedRules(w, catalog.Rules)
}

// renderHandlingDefinitions 输出处理方式词典，帮助读者理解 handling 字段。
func renderHandlingDefinitions(w io.Writer) error {
	if _, err := fmt.Fprint(w, "\n## 处理方式定义\n\n"); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(w, "catalog 不使用“工具 + 人工混合覆盖”这类模糊状态。原则是：只有确定能由工具稳定处理的规则，才标为工具处理；否则退一步，进入更宽松的 LLM/人工 review 或 candidate。"); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(w, "\n| 处理方式 | 含义 | go-review 处理方式 |"); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(w, "| --- | --- | --- |"); err != nil {
		return err
	}
	rows := [][3]string{
		{"tool-golangci", "已确认可由 golangci-lint 内置 linter/formatter 稳定处理", "通过 `go.lint` adapter 启用，不自研。"},
		{"tool-golangci-config", "可由 golangci-lint 处理，但必须配置阈值、名单、例外或具体 linter", "先沉淀推荐配置，再按 profile 启用。"},
		{"tool-go-test", "通过 `go test`、example test 或测试运行本身验证", "保持独立 `go.test` step。"},
		{"tool-external", "更适合独立工具，例如 `govulncheck`、`gosec`", "独立 adapter 或可选 profile。"},
		{"tool-semantic", "已确认需要 AST/type info，且能写成稳定 `go.semantic` analyzer", "进入 `.go-review/rules/semantic-custom.yaml` 支持范围或新增 analyzer。"},
		{"llm-review", "需要上下文判断、原则解释或设计取舍；不适合确定性工具 gate", "写入规范文档，并进入 LLM report / 人工 review checklist；默认不阻断。"},
		{"candidate", "候选规则，仍需验证工具覆盖、误判率和启用成本", "不默认启用。"},
	}
	for _, row := range rows {
		if _, err := fmt.Fprintf(w, "| `%s` | %s | %s |\n", row[0], row[1], row[2]); err != nil {
			return err
		}
	}
	return nil
}

// renderDefaultProfiles 输出推荐启用层级，说明哪些规则适合 default/ci/strict/nightly。
func renderDefaultProfiles(w io.Writer) error {
	if _, err := fmt.Fprint(w, "\n## 默认启用策略\n\n"); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(w, "| 层级 | 默认策略 | 例子 |"); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(w, "| --- | --- | --- |"); err != nil {
		return err
	}
	rows := [][3]string{
		{"default", "低争议、确定性强、误判低", "gofmt、goimports、govet、staticcheck、unused、ineffassign、errcheck、go test。"},
		{"ci", "PR 可承受、结果稳定", "errorlint、bodyclose、contextcheck、基础 semantic 规则。"},
		{"strict", "团队明确接受后启用", "函数长度、复杂度、import 边界、变量名长度、接口方法数量。"},
		{"nightly", "慢速或全量扫描", "govulncheck、gosec 全量、race、重型 semantic。"},
		{"doc", "只解释，不阻断", "接口是否过早抽象、包边界是否清晰、代码是否足够简单。"},
	}
	for _, row := range rows {
		if _, err := fmt.Fprintf(w, "| `%s` | %s | %s |\n", row[0], row[1], row[2]); err != nil {
			return err
		}
	}
	return nil
}

// renderJSONContract 输出 catalog CRUD 使用方式和 report rule_id 追踪约束。
func renderJSONContract(w io.Writer) error {
	if _, err := fmt.Fprint(w, "\n## JSON catalog 与 CRUD\n\n"); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(w, "`rules/go-rules.json` 是机器可读源数据，`docs/quality/go-rule-catalog.md` 是面向人阅读的视图。检测报告必须全链路携带稳定 `rule_id`。`implemented` 必须显式标识该规则是否已经被代码实现并有测试覆盖；不能只因为进入 catalog 就算已实现。"); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(w, "\nCRUD 命令：\n\n```bash\ngo-review rules list --catalog rules/go-rules.json\ngo-review rules get team.semantic.max-params --catalog rules/go-rules.json\ngo-review rules add --catalog rules/go-rules.json --file new-rule.json\ngo-review rules upsert --catalog rules/go-rules.json --file changed-rule.json\ngo-review rules delete old.rule.id --catalog rules/go-rules.json\ngo-review rules validate --catalog rules/go-rules.json\ngo-review rules render-doc --catalog rules/go-rules.json --out docs/quality/go-rule-catalog.md\n```"); err != nil {
		return err
	}
	return nil
}

// renderGroupedRules 按实际落地路径分组输出规则。
// 这种视图比按来源分组更方便工程落地：能直接看到规则应该交给 lint、semantic、LLM
// 还是继续保留 candidate。
func renderGroupedRules(w io.Writer, rules []Rule) error {
	groups := []ruleGroup{
		{
			Key:         "lint",
			Title:       "A. 走 lint / 现成工具框架",
			SummaryName: "lint / 现成工具框架",
			Description: "这些规则应优先通过 `go.lint`、`go.test` 或独立外部 adapter 承接。`tool-golangci-config` 表示需要先固化 linter、阈值、例外或团队配置。",
			Principle:   "优先复用 `golangci-lint`、`go test`、`gosec`、`govulncheck` 等；能稳定映射 `rule_id` 后再标 `implemented: true`。",
		},
		{
			Key:         "semantic",
			Title:       "B. 走自定义开发 / go.semantic",
			SummaryName: "自定义开发 / go.semantic",
			Description: "这些规则适合开发或扩展 `go.semantic` analyzer。只有能稳定从 AST、import、type info 或确定性控制流里判断的规则才放这里。",
			Principle:   "现成工具覆盖不了，但 AST、import、type info 或确定性控制流能稳定判断时，开发 `go/analysis` analyzer。",
		},
		{
			Key:         "llm",
			Title:       "C. 走 LLM / 人工 review",
			SummaryName: "LLM / 人工 review",
			Description: "这些规则需要上下文和设计判断。go-review 应把它们写入规范文档、LLM 报告和人工 checklist，但默认不作为确定性 gate。",
			Principle:   "需要设计意图、API 语义、并发生命周期、测试诊断质量或项目上下文；进入报告和 checklist，默认不硬阻断。",
		},
		{
			Key:         "candidate",
			Title:       "D. Candidate：待确认后再归类",
			SummaryName: "Candidate",
			Description: "这些规则暂不标实现。下一步要么证明能由 lint/semantic 稳定处理并迁入 A/B，要么降级为 LLM review。",
			Principle:   "待验证底层工具能力、误判率、配置成本和是否值得阻断；确认后迁入前三类。",
		},
	}
	byGroup := map[string][]Rule{}
	for _, rule := range rules {
		key := ruleGroupKey(rule)
		byGroup[key] = append(byGroup[key], rule)
	}
	if _, err := fmt.Fprint(w, "\n## 按落地路径分组\n\n"); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(w, "本 catalog 的主维护视图按 go-review 的实际落地路径分组，而不是只按规范来源展开。这样能直接看出一条规则应该交给现成工具、自定义 analyzer，还是 LLM/人工 review。"); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(w, "\n| 分组 | 数量 | 处理原则 |"); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(w, "| --- | ---: | --- |"); err != nil {
		return err
	}
	for _, group := range groups {
		if _, err := fmt.Fprintf(w, "| %s | %d | %s |\n", group.SummaryName, len(byGroup[group.Key]), group.Principle); err != nil {
			return err
		}
	}
	for _, group := range groups {
		if err := renderRuleGroup(w, group, byGroup[group.Key]); err != nil {
			return err
		}
	}
	return nil
}

// ruleGroup 描述 Markdown 中一个规则分组的展示信息。
type ruleGroup struct {
	Key         string
	Title       string
	SummaryName string
	Description string
	Principle   string
}

// ruleGroupKey 根据 handling 把规则归入四个落地分组。
func ruleGroupKey(rule Rule) string {
	switch rule.Handling {
	case "tool-golangci", "tool-golangci-config", "tool-go-test", "tool-external":
		return "lint"
	case "tool-semantic":
		return "semantic"
	case "llm-review":
		return "llm"
	case "candidate":
		return "candidate"
	default:
		return "candidate"
	}
}

// renderRuleGroup 输出单个分组下的规则表格。
func renderRuleGroup(w io.Writer, group ruleGroup, rules []Rule) error {
	if _, err := fmt.Fprintf(w, "\n### %s\n\n%s\n\n", group.Title, group.Description); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(w, "| Rule ID | 规则说明 | 处理方式 | 推荐承接 | 默认 | 已实现 | 备注 |"); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(w, "| --- | --- | --- | --- | --- | --- | --- |"); err != nil {
		return err
	}
	for _, rule := range rules {
		if _, err := fmt.Fprintf(w, "| `%s` | %s | `%s` | %s | %s | %s | %s |\n", md(rule.ID), md(rule.Description), md(rule.Handling), md(ruleAdapterLabel(rule)), md(dash(rule.DefaultProfile)), md(implementedLabel(rule.Implemented)), md(rule.Notes)); err != nil {
			return err
		}
	}
	return nil
}

// ruleAdapterLabel 生成人类可读的承接工具标签。
func ruleAdapterLabel(rule Rule) string {
	adapter := strings.TrimSpace(rule.Adapter)
	if len(rule.ToolRules) > 0 {
		if adapter != "" {
			adapter += " / "
		}
		adapter += strings.Join(rule.ToolRules, ", ")
	}
	if adapter != "" {
		return adapter
	}
	if rule.Handling == "candidate" {
		return "候选待确认"
	}
	return "LLM/人工 review"
}

// normalizeStrings 去空白、去重并排序字符串列表，保证 JSON 输出稳定。
func normalizeStrings(values []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}

// validHandling 限定 catalog 允许的处理方式枚举。
func validHandling(value string) bool {
	switch value {
	case "tool-golangci", "tool-golangci-config", "tool-go-test", "tool-external", "tool-semantic", "llm-review", "candidate":
		return true
	default:
		return false
	}
}

// validStatus 限定规则生命周期状态枚举。
func validStatus(value string) bool {
	switch value {
	case "active", "candidate", "deprecated":
		return true
	default:
		return false
	}
}

// validAutofixSafety 限定自动修复安全等级枚举。
func validAutofixSafety(value string) bool {
	switch value {
	case "", "safe", "review", "none":
		return true
	default:
		return false
	}
}

// md 转义 Markdown 表格中的特殊字符，并把空值展示为 `-`。
func md(value string) string {
	value = strings.ReplaceAll(value, "|", "\\|")
	value = strings.ReplaceAll(value, "\n", "<br>")
	value = strings.TrimSpace(value)
	if value == "" {
		return "-"
	}
	return value
}

// dash 把空字符串转换为表格占位符。
func dash(value string) string {
	if strings.TrimSpace(value) == "" {
		return "-"
	}
	return value
}

// implementedLabel 把布尔实现状态转成人类可读标签。
func implementedLabel(implemented bool) string {
	if implemented {
		return "yes"
	}
	return "no"
}
