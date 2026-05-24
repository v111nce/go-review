package engine

import (
	"context"
	"fmt"
	"go/ast"
	"go/importer"
	"go/parser"
	"go/token"
	"go/types"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"

	"golang.org/x/tools/go/analysis"

	"github.com/v111nce/go-review/internal/config"
)

const noDirectEnvRuleID = "semantic.no-direct-os-getenv"

// SemanticAdapter runs configured Go semantic rules as go/analysis analyzers.
type SemanticAdapter struct {
	cfg config.Adapter
}

type semanticAnalyzerFactory func() (*analysis.Analyzer, semanticRuleMeta)
type semanticCustomAnalyzerFactory func(semanticCustomRule) (*analysis.Analyzer, semanticRuleMeta)

type semanticRuleMeta struct {
	RuleID        string
	PassMessage   string
	FixSafety     config.FixSafety
	FixAvailable  bool
	SuggestionFor func(analysis.Diagnostic) string
}

var semanticBuiltInAnalyzers = map[string]semanticAnalyzerFactory{
	"no-direct-os-getenv": func() (*analysis.Analyzer, semanticRuleMeta) {
		meta := semanticRuleMeta{
			RuleID:      noDirectEnvRuleID,
			PassMessage: "semantic no-direct-os-getenv passed",
			SuggestionFor: func(analysis.Diagnostic) string {
				return "read environment values through an injected config/env provider"
			},
		}
		return noDirectCallAnalyzer(semanticCustomRule{
			ID:         noDirectEnvRuleID,
			Package:    "os",
			Function:   "Getenv",
			Message:    "direct os.Getenv access bypasses injectable configuration",
			Suggestion: "read environment values through an injected config/env provider",
		}, "no_direct_os_getenv"), meta
	},
}

var semanticCustomAnalyzers = map[string]semanticCustomAnalyzerFactory{
	"no-direct-call": func(rule semanticCustomRule) (*analysis.Analyzer, semanticRuleMeta) {
		message := rule.Message
		if message == "" {
			message = fmt.Sprintf("direct %s.%s call is disallowed", rule.Package, rule.Function)
		}
		meta := semanticRuleMeta{
			RuleID:      semanticRuleID(rule.ID),
			PassMessage: fmt.Sprintf("semantic custom rule %s passed", rule.ID),
			SuggestionFor: func(analysis.Diagnostic) string {
				return rule.Suggestion
			},
		}
		rule.Message = message
		return noDirectCallAnalyzer(rule, analyzerName(rule.ID)), meta
	},
	"max-params": func(rule semanticCustomRule) (*analysis.Analyzer, semanticRuleMeta) {
		meta := semanticRuleMeta{
			RuleID:      semanticRuleID(rule.ID),
			PassMessage: fmt.Sprintf("semantic custom rule %s passed", rule.ID),
			SuggestionFor: func(analysis.Diagnostic) string {
				return rule.Suggestion
			},
		}
		return maxParamsAnalyzer(rule, analyzerName(rule.ID)), meta
	},
}

func (a SemanticAdapter) Metadata() AdapterMetadata {
	return AdapterMetadata{ID: a.cfg.ID, Type: "go.semantic", Capabilities: a.cfg.Capabilities, Version: a.cfg.Version}
}

func (a SemanticAdapter) Run(_ context.Context, stepCtx StepContext) (Result, error) {
	cfg, err := a.semanticConfig(stepCtx)
	if err != nil {
		return Result{AdapterID: a.cfg.ID, StepID: stepCtx.Step.ID, RuleID: "semantic.config", Kind: ResultViolation, Message: err.Error(), FixSafety: config.FixNone, GateStatus: config.GateFail}, nil
	}
	for _, rule := range cfg.BuiltInRules {
		analyzer, meta, err := a.builtInAnalyzer(rule)
		if err != nil {
			return a.semanticConfigFailure(stepCtx, err.Error()), nil
		}
		result, err := a.runAnalyzer(analyzer, meta, stepCtx)
		if err != nil || result.GateStatus == config.GateFail {
			return result, err
		}
	}
	for _, rule := range cfg.CustomRules {
		analyzer, meta, err := a.customAnalyzer(rule)
		if err != nil {
			return a.semanticConfigFailure(stepCtx, err.Error()), nil
		}
		result, err := a.runAnalyzer(analyzer, meta, stepCtx)
		if err != nil || result.GateStatus == config.GateFail {
			return result, err
		}
	}
	ruleCount := len(cfg.BuiltInRules) + len(cfg.CustomRules)
	return Result{AdapterID: a.cfg.ID, StepID: stepCtx.Step.ID, RuleID: "semantic.rules", Kind: ResultArtifact, Message: fmt.Sprintf("semantic analyzers passed (%d)", ruleCount), FixSafety: a.cfg.FixSafety, GateStatus: config.GatePass}, nil
}

func (a SemanticAdapter) builtInAnalyzer(rule string) (*analysis.Analyzer, semanticRuleMeta, error) {
	rule = semanticBuiltInRuleName(rule)
	factory, ok := semanticBuiltInAnalyzers[rule]
	if !ok {
		return nil, semanticRuleMeta{}, fmt.Errorf("unsupported semantic rule %q", rule)
	}
	analyzer, meta := factory()
	return analyzer, meta, nil
}

func (a SemanticAdapter) customAnalyzer(rule semanticCustomRule) (*analysis.Analyzer, semanticRuleMeta, error) {
	kind := semanticCustomKind(rule.Kind)
	factory, ok := semanticCustomAnalyzers[kind]
	if !ok {
		return nil, semanticRuleMeta{}, fmt.Errorf("unsupported semantic custom rule kind %q", kind)
	}
	rule.Kind = kind
	analyzer, meta := factory(rule)
	return analyzer, meta, nil
}

func maxParamsAnalyzer(rule semanticCustomRule, name string) *analysis.Analyzer {
	return &analysis.Analyzer{
		Name:             name,
		Doc:              fmt.Sprintf("reports functions with more than %d parameters", rule.Max),
		RunDespiteErrors: true,
		Run: func(pass *analysis.Pass) (any, error) {
			for _, file := range pass.Files {
				ast.Inspect(file, func(node ast.Node) bool {
					decl, ok := node.(*ast.FuncDecl)
					if !ok || decl.Type == nil {
						return true
					}
					count := fieldListNameCount(decl.Type.Params)
					if count <= rule.Max {
						return false
					}
					message := rule.Message
					if message == "" {
						message = fmt.Sprintf("function %s has %d parameters, maximum is %d", decl.Name.Name, count, rule.Max)
					}
					diagnostic := analysis.Diagnostic{Pos: decl.Pos(), Category: semanticRuleID(rule.ID), Message: message}
					if rule.Suggestion != "" {
						diagnostic.SuggestedFixes = []analysis.SuggestedFix{{Message: rule.Suggestion}}
					}
					pass.Report(diagnostic)
					return false
				})
			}
			return nil, nil
		},
	}
}

func fieldListNameCount(fields *ast.FieldList) int {
	if fields == nil {
		return 0
	}
	count := 0
	for _, field := range fields.List {
		if len(field.Names) == 0 {
			count++
			continue
		}
		count += len(field.Names)
	}
	return count
}

func noDirectCallAnalyzer(rule semanticCustomRule, name string) *analysis.Analyzer {
	return &analysis.Analyzer{
		Name:             name,
		Doc:              fmt.Sprintf("reports direct %s.%s calls", rule.Package, rule.Function),
		RunDespiteErrors: true,
		Run: func(pass *analysis.Pass) (any, error) {
			for _, file := range pass.Files {
				packageNames := importedNames(file, rule.Package)
				ast.Inspect(file, func(node ast.Node) bool {
					call, ok := node.(*ast.CallExpr)
					if !ok {
						return true
					}
					sel, ok := call.Fun.(*ast.SelectorExpr)
					if !ok || sel.Sel == nil || sel.Sel.Name != rule.Function {
						return true
					}
					if !isPackageFunctionSelector(pass.TypesInfo, packageNames, rule.Package, rule.Function, sel) {
						return true
					}
					diagnostic := analysis.Diagnostic{Pos: call.Pos(), Category: semanticRuleID(rule.ID), Message: rule.Message}
					if rule.Suggestion != "" {
						diagnostic.SuggestedFixes = []analysis.SuggestedFix{{Message: rule.Suggestion}}
					}
					pass.Report(diagnostic)
					return true
				})
			}
			return nil, nil
		},
	}
}

func (a SemanticAdapter) runAnalyzer(analyzer *analysis.Analyzer, meta semanticRuleMeta, stepCtx StepContext) (Result, error) {
	files, err := goFiles(resolveWorkdir(stepCtx.ProjectRoot, a.cfg.Workdir), projectExcludes(stepCtx.Config))
	if err != nil {
		return Result{}, err
	}
	if len(files) == 0 {
		return Result{AdapterID: a.cfg.ID, StepID: stepCtx.Step.ID, RuleID: meta.RuleID, Kind: ResultArtifact, Message: fmt.Sprintf("%s skipped; no non-excluded Go files", analyzer.Name), FixSafety: a.cfg.FixSafety, GateStatus: config.GatePass}, nil
	}
	fset := token.NewFileSet()
	parsedFiles := make([]*ast.File, 0, len(files))
	for _, file := range files {
		parsed, err := parser.ParseFile(fset, file, nil, 0)
		if err != nil {
			return Result{AdapterID: a.cfg.ID, StepID: stepCtx.Step.ID, RuleID: meta.RuleID, Kind: ResultViolation, File: relPath(stepCtx.ProjectRoot, file), Message: err.Error(), FixSafety: config.FixNone, GateStatus: config.GateFail}, nil
		}
		parsedFiles = append(parsedFiles, parsed)
	}
	info := &types.Info{Uses: map[*ast.Ident]types.Object{}}
	typesCfg := types.Config{Importer: importer.Default(), Error: func(error) {}}
	pkg, _ := typesCfg.Check("semanticfixture", fset, parsedFiles, info)

	var diagnostics []analysis.Diagnostic
	pass := &analysis.Pass{
		Analyzer:  analyzer,
		Fset:      fset,
		Files:     parsedFiles,
		Pkg:       pkg,
		TypesInfo: info,
		Report: func(diagnostic analysis.Diagnostic) {
			diagnostics = append(diagnostics, diagnostic)
		},
		ReadFile: os.ReadFile,
	}
	if _, err := analyzer.Run(pass); err != nil {
		return Result{}, err
	}
	if len(diagnostics) == 0 {
		return Result{AdapterID: a.cfg.ID, StepID: stepCtx.Step.ID, RuleID: meta.RuleID, Kind: ResultArtifact, Message: meta.PassMessage, FixSafety: a.cfg.FixSafety, GateStatus: config.GatePass}, nil
	}
	diagnostic := diagnostics[0]
	pos := fset.Position(diagnostic.Pos)
	return Result{
		AdapterID:    a.cfg.ID,
		StepID:       stepCtx.Step.ID,
		RuleID:       diagnosticRuleID(diagnostic, meta.RuleID),
		Kind:         ResultViolation,
		File:         relPath(stepCtx.ProjectRoot, pos.Filename),
		Line:         pos.Line,
		Column:       pos.Column,
		Message:      diagnostic.Message,
		Suggestion:   diagnosticSuggestion(diagnostic, meta),
		FixAvailable: meta.FixAvailable,
		FixSafety:    a.cfg.FixSafety,
		GateStatus:   config.GateFail,
	}, nil
}

func diagnosticRuleID(diagnostic analysis.Diagnostic, fallback string) string {
	if strings.TrimSpace(diagnostic.Category) != "" {
		return semanticRuleID(diagnostic.Category)
	}
	return fallback
}

func diagnosticSuggestion(diagnostic analysis.Diagnostic, meta semanticRuleMeta) string {
	if meta.SuggestionFor != nil {
		if suggestion := meta.SuggestionFor(diagnostic); suggestion != "" {
			return suggestion
		}
	}
	for _, fix := range diagnostic.SuggestedFixes {
		if strings.TrimSpace(fix.Message) != "" {
			return fix.Message
		}
	}
	return ""
}

func semanticBuiltInRuleName(rule string) string {
	rule = strings.TrimSpace(rule)
	if rule == "" {
		return "no-direct-os-getenv"
	}
	return rule
}

type semanticConfig struct {
	BuiltInRules []string
	CustomRules  []semanticCustomRule
}

type semanticCustomRule struct {
	ID         string
	Kind       string
	Package    string
	Function   string
	Max        int
	Message    string
	Suggestion string
}

func (a SemanticAdapter) semanticConfig(stepCtx StepContext) (semanticConfig, error) {
	if strings.TrimSpace(a.cfg.Parser) != "" {
		return semanticConfig{}, fmt.Errorf("go.semantic does not support adapter parser; configure built-in rules in semantic/default.yaml")
	}
	configDir := filepath.Dir(stepCtx.ConfigPath)
	paths := []struct {
		path string
		kind semanticConfigKind
	}{
		{path: filepath.Join(configDir, "semantic", "default.yaml"), kind: semanticConfigBuiltInRules},
		{path: filepath.Join(configDir, "semantic", "custom.yaml"), kind: semanticConfigCustomRules},
	}
	var merged semanticConfig
	for _, source := range paths {
		loaded, err := loadSemanticConfig(source.path, source.kind)
		if err != nil {
			return semanticConfig{}, err
		}
		merged.BuiltInRules = append(merged.BuiltInRules, loaded.BuiltInRules...)
		merged.CustomRules = append(merged.CustomRules, loaded.CustomRules...)
	}
	if len(merged.BuiltInRules) == 0 && len(merged.CustomRules) == 0 {
		merged.BuiltInRules = []string{"no-direct-os-getenv"}
	}
	return merged, nil
}

type semanticConfigKind int

const (
	semanticConfigBuiltInRules semanticConfigKind = iota
	semanticConfigCustomRules
)

func loadSemanticConfig(path string, kind semanticConfigKind) (semanticConfig, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return semanticConfig{}, nil
	}
	if err != nil {
		return semanticConfig{}, err
	}
	lines, err := readSemanticConfigLines(path, data)
	if err != nil {
		return semanticConfig{}, err
	}
	var cfg semanticConfig
	for i := 0; i < len(lines); i++ {
		line := lines[i]
		if line.indent != 0 {
			return semanticConfig{}, fmt.Errorf("%s:%d: expected top-level semantic config field", path, line.num)
		}
		key, values, ok := parseSemanticListHeader(line.text)
		if !ok {
			return semanticConfig{}, fmt.Errorf("%s:%d: expected semantic config field", path, line.num)
		}
		switch key {
		case "rules":
			if kind == semanticConfigBuiltInRules {
				cfg.BuiltInRules = append(cfg.BuiltInRules, values...)
				if len(values) == 0 {
					more, next, err := parseSemanticBuiltInRuleItems(path, lines, i+1, line.indent)
					if err != nil {
						return semanticConfig{}, err
					}
					cfg.BuiltInRules = append(cfg.BuiltInRules, more...)
					i = next - 1
				}
				continue
			}
			if len(values) != 0 {
				return semanticConfig{}, fmt.Errorf("%s:%d: custom semantic rules must be a block list", path, line.num)
			}
			rules, next, err := parseSemanticCustomRuleItems(path, lines, i+1, line.indent)
			if err != nil {
				return semanticConfig{}, err
			}
			cfg.CustomRules = append(cfg.CustomRules, rules...)
			i = next - 1
		default:
			return semanticConfig{}, fmt.Errorf("%s:%d: unsupported semantic config field %q", path, line.num, key)
		}
	}
	return cfg, nil
}

type semanticConfigLine struct {
	num    int
	indent int
	text   string
}

func readSemanticConfigLines(path string, data []byte) ([]semanticConfigLine, error) {
	var lines []semanticConfigLine
	for lineNo, raw := range strings.Split(string(data), "\n") {
		raw = stripSemanticComment(raw)
		if strings.TrimSpace(raw) == "" {
			continue
		}
		if strings.Contains(raw, "\t") {
			return nil, fmt.Errorf("%s:%d: tabs are not supported in YAML indentation", path, lineNo+1)
		}
		indent := len(raw) - len(strings.TrimLeft(raw, " "))
		lines = append(lines, semanticConfigLine{num: lineNo + 1, indent: indent, text: strings.TrimSpace(raw)})
	}
	return lines, nil
}

func parseSemanticBuiltInRuleItems(path string, lines []semanticConfigLine, start, parentIndent int) ([]string, int, error) {
	var rules []string
	i := start
	for i < len(lines) && lines[i].indent > parentIndent {
		line := lines[i]
		if !strings.HasPrefix(line.text, "- ") {
			return nil, i, fmt.Errorf("%s:%d: expected built-in rules list item", path, line.num)
		}
		value := cleanSemanticValue(strings.TrimSpace(strings.TrimPrefix(line.text, "- ")))
		if value != "" {
			rules = append(rules, value)
		}
		i++
	}
	return rules, i, nil
}

func parseSemanticCustomRuleItems(path string, lines []semanticConfigLine, start, parentIndent int) ([]semanticCustomRule, int, error) {
	var rules []semanticCustomRule
	i := start
	for i < len(lines) && lines[i].indent > parentIndent {
		line := lines[i]
		if !strings.HasPrefix(line.text, "- ") {
			return nil, i, fmt.Errorf("%s:%d: expected custom rules list item", path, line.num)
		}
		rule := semanticCustomRule{}
		next, err := parseSemanticCustomRuleItem(path, lines, i, &rule)
		if err != nil {
			return nil, i, err
		}
		if err := validateSemanticCustomRule(path, line.num, rule); err != nil {
			return nil, i, err
		}
		rules = append(rules, rule)
		i = next
	}
	return rules, i, nil
}

func parseSemanticCustomRuleItem(path string, lines []semanticConfigLine, start int, rule *semanticCustomRule) (int, error) {
	itemIndent := lines[start].indent
	fields := []semanticConfigLine{{num: lines[start].num, indent: itemIndent + 2, text: strings.TrimSpace(strings.TrimPrefix(lines[start].text, "- "))}}
	i := start + 1
	for i < len(lines) && lines[i].indent > itemIndent {
		fields = append(fields, lines[i])
		i++
	}
	for _, field := range fields {
		if field.text == "" {
			continue
		}
		key, val, ok := parseSemanticField(field.text)
		if !ok {
			return i, fmt.Errorf("%s:%d: expected custom rule field", path, field.num)
		}
		switch key {
		case "id":
			rule.ID = cleanSemanticValue(val)
		case "kind":
			rule.Kind = cleanSemanticValue(val)
		case "package", "pkg":
			rule.Package = cleanSemanticValue(val)
		case "function", "func":
			rule.Function = cleanSemanticValue(val)
		case "max":
			parsed, err := parseSemanticPositiveInt(val)
			if err != nil {
				return i, fmt.Errorf("%s:%d: max must be a positive integer", path, field.num)
			}
			rule.Max = parsed
		case "message":
			rule.Message = cleanSemanticValue(val)
		case "suggestion":
			rule.Suggestion = cleanSemanticValue(val)
		default:
			return i, fmt.Errorf("%s:%d: unsupported custom rule field %q", path, field.num, key)
		}
	}
	return i, nil
}

func validateSemanticCustomRule(path string, lineNo int, rule semanticCustomRule) error {
	if strings.TrimSpace(rule.ID) == "" {
		return fmt.Errorf("%s:%d: custom rule missing id", path, lineNo)
	}
	kind := semanticCustomKind(rule.Kind)
	if _, ok := semanticCustomAnalyzers[kind]; !ok {
		return fmt.Errorf("%s:%d: unsupported custom rule kind %q", path, lineNo, kind)
	}
	switch kind {
	case "no-direct-call":
		if strings.TrimSpace(rule.Package) == "" || strings.TrimSpace(rule.Function) == "" {
			return fmt.Errorf("%s:%d: custom no-direct-call rule requires package and function", path, lineNo)
		}
	case "max-params":
		if rule.Max <= 0 {
			return fmt.Errorf("%s:%d: custom max-params rule requires positive max", path, lineNo)
		}
	}
	return nil
}

func parseSemanticPositiveInt(value string) (int, error) {
	value = cleanSemanticValue(value)
	n, err := strconv.Atoi(value)
	if err != nil || n <= 0 {
		return 0, fmt.Errorf("invalid positive integer %q", value)
	}
	return n, nil
}

func parseSemanticField(text string) (string, string, bool) {
	idx := strings.Index(text, ":")
	if idx < 0 {
		return "", "", false
	}
	return strings.TrimSpace(text[:idx]), strings.TrimSpace(text[idx+1:]), true
}

func parseSemanticListHeader(line string) (string, []string, bool) {
	key, rest, ok := strings.Cut(line, ":")
	if !ok {
		return "", nil, false
	}
	key = strings.TrimSpace(key)
	rest = strings.TrimSpace(rest)
	if rest == "" {
		return key, nil, true
	}
	if strings.HasPrefix(rest, "[") && strings.HasSuffix(rest, "]") {
		inner := strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(rest, "["), "]"))
		if inner == "" {
			return key, nil, true
		}
		parts := strings.Split(inner, ",")
		values := make([]string, 0, len(parts))
		for _, part := range parts {
			if value := cleanSemanticValue(part); value != "" {
				values = append(values, value)
			}
		}
		return key, values, true
	}
	if value := cleanSemanticValue(rest); value != "" {
		return key, []string{value}, true
	}
	return key, nil, true
}

func cleanSemanticValue(value string) string {
	return strings.Trim(strings.TrimSpace(value), "\"'")
}

func stripSemanticComment(s string) string {
	inSingle, inDouble := false, false
	for i, r := range s {
		switch r {
		case '\'':
			if !inDouble {
				inSingle = !inSingle
			}
		case '"':
			if !inSingle {
				inDouble = !inDouble
			}
		case '#':
			if !inSingle && !inDouble {
				return s[:i]
			}
		}
	}
	return s
}

func semanticCustomKind(kind string) string {
	kind = strings.TrimSpace(kind)
	if kind == "" {
		return "no-direct-call"
	}
	return kind
}

func (a SemanticAdapter) semanticConfigFailure(stepCtx StepContext, message string) Result {
	return Result{AdapterID: a.cfg.ID, StepID: stepCtx.Step.ID, RuleID: "semantic.config", Kind: ResultViolation, Message: message, FixSafety: config.FixNone, GateStatus: config.GateFail}
}

func semanticRuleID(id string) string {
	id = strings.TrimSpace(id)
	if id == "" {
		return "semantic.custom"
	}
	if strings.Contains(id, ".") {
		return id
	}
	return "semantic." + id
}

func analyzerName(id string) string {
	id = strings.TrimPrefix(semanticRuleID(id), "semantic.")
	var b strings.Builder
	for _, r := range id {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' {
			b.WriteRune(r)
		} else {
			b.WriteRune('_')
		}
	}
	name := strings.Trim(b.String(), "_")
	if name == "" || (name[0] >= '0' && name[0] <= '9') {
		name = "semantic_" + name
	}
	return name
}

func isPackageFunctionSelector(info *types.Info, packageNames map[string]struct{}, packagePath, function string, sel *ast.SelectorExpr) bool {
	if info != nil {
		obj := info.Uses[sel.Sel]
		if obj != nil && obj.Pkg() != nil {
			return obj.Pkg().Path() == packagePath && obj.Name() == function
		}
	}
	ident, ok := sel.X.(*ast.Ident)
	if !ok {
		return false
	}
	_, ok = packageNames[ident.Name]
	return ok
}

func importedNames(file *ast.File, importPath string) map[string]struct{} {
	names := map[string]struct{}{}
	for _, spec := range file.Imports {
		if strings.Trim(spec.Path.Value, "\"") != importPath {
			continue
		}
		if spec.Name != nil {
			if spec.Name.Name == "." || spec.Name.Name == "_" {
				continue
			}
			names[spec.Name.Name] = struct{}{}
			continue
		}
		names[path.Base(importPath)] = struct{}{}
	}
	return names
}
