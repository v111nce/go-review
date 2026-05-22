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
	"sort"
	"strings"

	"github.com/v111nce/go-review/internal/config"
)

const noDirectEnvRuleID = "semantic.no-direct-os-getenv"

// SemanticAdapter provides a first built-in go/analysis-style semantic rule bridge.
//
// It intentionally ships one small example rule instead of a dynamic plugin loader:
// direct os.Getenv calls are reported so teams can see how AST-backed rules map into
// the same pipeline/result contract without the core becoming a string-matching linter.
type SemanticAdapter struct {
	cfg config.Adapter
}

// These private runner maps are an internal seam only: they keep the current
// narrow semantic rules from piling up in switch statements without creating a
// public plugin, extension, SDK, or YAML DSL surface.
type semanticBuiltInRuleRunner func(SemanticAdapter, StepContext) (Result, error)

type semanticCustomKindRunner func(SemanticAdapter, semanticCustomRule, StepContext) (Result, error)

var semanticBuiltInRunners = map[string]semanticBuiltInRuleRunner{
	"no-direct-os-getenv": SemanticAdapter.runNoDirectOSGetenv,
}

var semanticCustomKindRunners = map[string]semanticCustomKindRunner{
	"no-direct-call": SemanticAdapter.runNoDirectCall,
}

func (a SemanticAdapter) Metadata() AdapterMetadata {
	return AdapterMetadata{ID: a.cfg.ID, Type: "go.semantic", Capabilities: a.cfg.Capabilities, Version: a.cfg.Version}
}

func (a SemanticAdapter) Run(_ context.Context, stepCtx StepContext) (Result, error) {
	cfg, err := a.semanticConfig(stepCtx)
	if err != nil {
		return Result{AdapterID: a.cfg.ID, StepID: stepCtx.Step.ID, RuleID: "semantic.config", Kind: ResultViolation, Message: err.Error(), FixSafety: config.FixNone, GateStatus: config.GateFail}, nil
	}
	for _, rule := range cfg.Rules {
		result, err := a.runRule(rule, stepCtx)
		if err != nil || result.GateStatus == config.GateFail {
			return result, err
		}
	}
	for _, rule := range cfg.CustomRules {
		result, err := a.runCustomRule(rule, stepCtx)
		if err != nil || result.GateStatus == config.GateFail {
			return result, err
		}
	}
	ruleCount := len(cfg.Rules) + len(cfg.CustomRules)
	return Result{
		AdapterID:  a.cfg.ID,
		StepID:     stepCtx.Step.ID,
		RuleID:     "semantic.rules",
		Kind:       ResultArtifact,
		Message:    fmt.Sprintf("semantic rules passed (%d)", ruleCount),
		FixSafety:  a.cfg.FixSafety,
		GateStatus: config.GatePass,
	}, nil
}

func (a SemanticAdapter) runRule(rule string, stepCtx StepContext) (Result, error) {
	rule = semanticBuiltInRuleName(rule)
	runner, ok := semanticBuiltInRunners[rule]
	if !ok {
		return a.semanticConfigFailure(stepCtx, fmt.Sprintf("unsupported semantic rule %q", rule)), nil
	}
	return runner(a, stepCtx)
}

func semanticBuiltInRuleName(rule string) string {
	rule = strings.TrimSpace(rule)
	if rule == "" {
		return "no-direct-os-getenv"
	}
	return rule
}

type semanticConfig struct {
	Rules       []string
	CustomRules []semanticCustomRule
}

type semanticCustomRule struct {
	ID         string
	Kind       string
	Package    string
	Function   string
	Message    string
	Suggestion string
}

func (a SemanticAdapter) semanticConfig(stepCtx StepContext) (semanticConfig, error) {
	// Parser is a backward-compatible selector for one built-in semantic rule;
	// it is not a parser plugin or extension mechanism.
	if strings.TrimSpace(a.cfg.Parser) != "" {
		return semanticConfig{Rules: []string{strings.TrimSpace(a.cfg.Parser)}}, nil
	}
	paths := []string{
		filepath.Join(filepath.Dir(stepCtx.ConfigPath), "semantic", "default.yaml"),
		filepath.Join(filepath.Dir(stepCtx.ConfigPath), "semantic", "custom.yaml"),
	}
	var merged semanticConfig
	for _, path := range paths {
		loaded, err := loadSemanticConfig(path)
		if err != nil {
			return semanticConfig{}, err
		}
		merged.Rules = append(merged.Rules, loaded.Rules...)
		merged.CustomRules = append(merged.CustomRules, loaded.CustomRules...)
	}
	if len(merged.Rules) == 0 && len(merged.CustomRules) == 0 {
		merged.Rules = []string{"no-direct-os-getenv"}
	}
	return merged, nil
}

func loadSemanticConfig(path string) (semanticConfig, error) {
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
			cfg.Rules = append(cfg.Rules, values...)
			if len(values) == 0 {
				more, next, err := parseSemanticRuleItems(path, lines, i+1, line.indent)
				if err != nil {
					return semanticConfig{}, err
				}
				cfg.Rules = append(cfg.Rules, more...)
				i = next - 1
			}
		case "custom_rules":
			if len(values) != 0 {
				return semanticConfig{}, fmt.Errorf("%s:%d: custom_rules must be a block list", path, line.num)
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

func parseSemanticRuleItems(path string, lines []semanticConfigLine, start, parentIndent int) ([]string, int, error) {
	var rules []string
	i := start
	for i < len(lines) && lines[i].indent > parentIndent {
		line := lines[i]
		if !strings.HasPrefix(line.text, "- ") {
			return nil, i, fmt.Errorf("%s:%d: expected rules list item", path, line.num)
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
			return nil, i, fmt.Errorf("%s:%d: expected custom_rules list item", path, line.num)
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
	if _, ok := semanticCustomKindRunners[kind]; !ok {
		return fmt.Errorf("%s:%d: unsupported custom rule kind %q", path, lineNo, kind)
	}
	if strings.TrimSpace(rule.Package) == "" || strings.TrimSpace(rule.Function) == "" {
		return fmt.Errorf("%s:%d: custom no-direct-call rule requires package and function", path, lineNo)
	}
	return nil
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

func defaultProjectExcludes() []string {
	return []string{".git", ".go-review", "artifacts", "vendor"}
}

func projectExcludes(cfg *config.Config) []string {
	if cfg == nil {
		return defaultProjectExcludes()
	}
	return append(defaultProjectExcludes(), cfg.Exclude...)
}

func normalizeProjectExcludes(values []string) []string {
	seen := map[string]struct{}{}
	var normalized []string
	for _, value := range values {
		value = filepath.ToSlash(strings.TrimSpace(value))
		value = strings.Trim(value, "/")
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		normalized = append(normalized, value)
	}
	return normalized
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

func (a SemanticAdapter) runCustomRule(rule semanticCustomRule, stepCtx StepContext) (Result, error) {
	kind := semanticCustomKind(rule.Kind)
	runner, ok := semanticCustomKindRunners[kind]
	if !ok {
		return a.semanticConfigFailure(stepCtx, fmt.Sprintf("unsupported semantic custom rule kind %q", kind)), nil
	}
	rule.Kind = kind
	return runner(a, rule, stepCtx)
}

func semanticCustomKind(kind string) string {
	kind = strings.TrimSpace(kind)
	if kind == "" {
		return "no-direct-call"
	}
	return kind
}

func (a SemanticAdapter) semanticConfigFailure(stepCtx StepContext, message string) Result {
	return Result{
		AdapterID:  a.cfg.ID,
		StepID:     stepCtx.Step.ID,
		RuleID:     "semantic.config",
		Kind:       ResultViolation,
		Message:    message,
		FixSafety:  config.FixNone,
		GateStatus: config.GateFail,
	}
}

func (a SemanticAdapter) runNoDirectCall(rule semanticCustomRule, stepCtx StepContext) (Result, error) {
	files, err := goFiles(resolveWorkdir(stepCtx.ProjectRoot, a.cfg.Workdir), projectExcludes(stepCtx.Config))
	if err != nil {
		return Result{}, err
	}
	fset := token.NewFileSet()
	parsedFiles := make([]*ast.File, 0, len(files))
	for _, file := range files {
		parsed, err := parser.ParseFile(fset, file, nil, 0)
		if err != nil {
			return Result{
				AdapterID:  a.cfg.ID,
				StepID:     stepCtx.Step.ID,
				RuleID:     semanticRuleID(rule.ID),
				Kind:       ResultViolation,
				File:       relPath(stepCtx.ProjectRoot, file),
				Message:    err.Error(),
				FixSafety:  config.FixNone,
				GateStatus: config.GateFail,
			}, nil
		}
		parsedFiles = append(parsedFiles, parsed)
	}

	info := &types.Info{Uses: map[*ast.Ident]types.Object{}}
	typesCfg := types.Config{Importer: importer.Default(), Error: func(error) {}}
	// Type-check errors are intentionally tolerated: package/type info improves
	// selector precision when available, and import-name fallback keeps this
	// narrow rule useful for partial or fixture packages.
	_, _ = typesCfg.Check("semanticcustom", fset, parsedFiles, info)

	for _, parsed := range parsedFiles {
		packageNames := importedNames(parsed, rule.Package)
		var finding *Result
		ast.Inspect(parsed, func(node ast.Node) bool {
			if finding != nil {
				return false
			}
			call, ok := node.(*ast.CallExpr)
			if !ok {
				return true
			}
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok || sel.Sel == nil || sel.Sel.Name != rule.Function {
				return true
			}
			if !isPackageFunctionSelector(info, packageNames, rule.Package, rule.Function, sel) {
				return true
			}
			pos := fset.Position(call.Pos())
			message := rule.Message
			if message == "" {
				message = fmt.Sprintf("direct %s.%s call is disallowed", rule.Package, rule.Function)
			}
			finding = &Result{
				AdapterID:    a.cfg.ID,
				StepID:       stepCtx.Step.ID,
				RuleID:       semanticRuleID(rule.ID),
				Kind:         ResultViolation,
				File:         relPath(stepCtx.ProjectRoot, pos.Filename),
				Line:         pos.Line,
				Column:       pos.Column,
				Message:      message,
				Suggestion:   rule.Suggestion,
				FixAvailable: false,
				FixSafety:    a.cfg.FixSafety,
				GateStatus:   config.GateFail,
			}
			return false
		})
		if finding != nil {
			return *finding, nil
		}
	}
	return Result{
		AdapterID:  a.cfg.ID,
		StepID:     stepCtx.Step.ID,
		RuleID:     semanticRuleID(rule.ID),
		Kind:       ResultArtifact,
		Message:    fmt.Sprintf("semantic custom rule %s passed", rule.ID),
		FixSafety:  a.cfg.FixSafety,
		GateStatus: config.GatePass,
	}, nil
}

func (a SemanticAdapter) runNoDirectOSGetenv(stepCtx StepContext) (Result, error) {
	files, err := goFiles(resolveWorkdir(stepCtx.ProjectRoot, a.cfg.Workdir), projectExcludes(stepCtx.Config))
	if err != nil {
		return Result{}, err
	}
	fset := token.NewFileSet()
	parsedFiles := make([]*ast.File, 0, len(files))
	for _, file := range files {
		parsed, err := parser.ParseFile(fset, file, nil, 0)
		if err != nil {
			return Result{
				AdapterID:  a.cfg.ID,
				StepID:     stepCtx.Step.ID,
				RuleID:     noDirectEnvRuleID,
				Kind:       ResultViolation,
				File:       relPath(stepCtx.ProjectRoot, file),
				Message:    err.Error(),
				FixSafety:  config.FixNone,
				GateStatus: config.GateFail,
			}, nil
		}
		parsedFiles = append(parsedFiles, parsed)
	}

	info := &types.Info{Uses: map[*ast.Ident]types.Object{}}
	typesCfg := types.Config{Importer: importer.Default(), Error: func(error) {}}
	// Type-check errors are intentionally tolerated; see runNoDirectCall for the
	// fallback contract shared by semantic rules.
	_, _ = typesCfg.Check("semanticfixture", fset, parsedFiles, info)

	var findings []Result
	for _, parsed := range parsedFiles {
		osNames := importedNames(parsed, "os")
		ast.Inspect(parsed, func(node ast.Node) bool {
			call, ok := node.(*ast.CallExpr)
			if !ok {
				return true
			}
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok || sel.Sel == nil || sel.Sel.Name != "Getenv" {
				return true
			}
			if !isOSGetenvSelector(info, osNames, sel) {
				return true
			}
			pos := fset.Position(call.Pos())
			findings = append(findings, Result{
				AdapterID:    a.cfg.ID,
				StepID:       stepCtx.Step.ID,
				RuleID:       noDirectEnvRuleID,
				Kind:         ResultViolation,
				File:         relPath(stepCtx.ProjectRoot, pos.Filename),
				Line:         pos.Line,
				Column:       pos.Column,
				Message:      "direct os.Getenv access bypasses injectable configuration",
				Suggestion:   "read environment values through an injected config/env provider",
				FixAvailable: false,
				FixSafety:    a.cfg.FixSafety,
				GateStatus:   config.GateFail,
			})
			return true
		})
	}
	if len(findings) == 0 {
		return Result{
			AdapterID:  a.cfg.ID,
			StepID:     stepCtx.Step.ID,
			RuleID:     noDirectEnvRuleID,
			Kind:       ResultArtifact,
			Message:    "semantic no-direct-os-getenv passed",
			FixSafety:  a.cfg.FixSafety,
			GateStatus: config.GatePass,
		}, nil
	}
	return findings[0], nil
}

func isOSGetenvSelector(info *types.Info, osNames map[string]struct{}, sel *ast.SelectorExpr) bool {
	obj := info.Uses[sel.Sel]
	if obj != nil && obj.Pkg() != nil {
		return obj.Pkg().Path() == "os" && obj.Name() == "Getenv"
	}
	ident, ok := sel.X.(*ast.Ident)
	if !ok {
		return false
	}
	_, ok = osNames[ident.Name]
	return ok
}

func semanticRuleID(id string) string {
	id = strings.TrimSpace(id)
	if id == "" {
		return "semantic.custom"
	}
	if strings.HasPrefix(id, "semantic.") {
		return id
	}
	return "semantic." + id
}

func isPackageFunctionSelector(info *types.Info, packageNames map[string]struct{}, packagePath, function string, sel *ast.SelectorExpr) bool {
	obj := info.Uses[sel.Sel]
	if obj != nil && obj.Pkg() != nil {
		return obj.Pkg().Path() == packagePath && obj.Name() == function
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
		// The selector name for an unaliased import is the package name, which is
		// usually the final path element (for example log/slog is used as slog.X).
		names[path.Base(importPath)] = struct{}{}
	}
	return names
}

func goFiles(root string, excludes []string) ([]string, error) {
	var files []string
	excludes = normalizeProjectExcludes(append(defaultProjectExcludes(), excludes...))
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			if projectPathExcluded(root, path, entry.Name(), excludes) {
				return filepath.SkipDir
			}
			return nil
		}
		if strings.HasSuffix(entry.Name(), ".go") && !projectPathExcluded(root, path, entry.Name(), excludes) {
			files = append(files, path)
		}
		return nil
	})
	sort.Strings(files)
	return files, err
}

func projectPathExcluded(root, path, name string, excludes []string) bool {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return false
	}
	rel = filepath.ToSlash(rel)
	if rel == "." {
		return false
	}
	for _, exclude := range excludes {
		if name == exclude || rel == exclude || strings.HasPrefix(rel, exclude+"/") {
			return true
		}
	}
	return false
}

func relPath(root, file string) string {
	if rel, err := filepath.Rel(root, file); err == nil && !strings.HasPrefix(rel, "..") {
		return rel
	}
	return file
}
