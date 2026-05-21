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

func (a SemanticAdapter) Metadata() AdapterMetadata {
	return AdapterMetadata{ID: a.cfg.ID, Type: "go.semantic", Capabilities: a.cfg.Capabilities, Version: a.cfg.Version}
}

func (a SemanticAdapter) Run(_ context.Context, stepCtx StepContext) (Result, error) {
	cfg, err := a.semanticConfig(stepCtx)
	if err != nil {
		return Result{AdapterID: a.cfg.ID, StepID: stepCtx.Step.ID, RuleID: "semantic.config", Kind: ResultViolation, Message: err.Error(), FixSafety: config.FixNone, GateStatus: config.GateFail}, nil
	}
	for _, rule := range cfg.Rules {
		result, err := a.runRule(rule, cfg.Exclude, stepCtx)
		if err != nil || result.GateStatus == config.GateFail {
			return result, err
		}
	}
	return Result{
		AdapterID:  a.cfg.ID,
		StepID:     stepCtx.Step.ID,
		RuleID:     "semantic.rules",
		Kind:       ResultArtifact,
		Message:    fmt.Sprintf("semantic rules passed (%d)", len(cfg.Rules)),
		FixSafety:  a.cfg.FixSafety,
		GateStatus: config.GatePass,
	}, nil
}

func (a SemanticAdapter) runRule(rule string, excludes []string, stepCtx StepContext) (Result, error) {
	switch rule {
	case "", "no-direct-os-getenv":
		return a.runNoDirectOSGetenv(excludes, stepCtx)
	default:
		return Result{
			AdapterID:  a.cfg.ID,
			StepID:     stepCtx.Step.ID,
			RuleID:     rule,
			Kind:       ResultViolation,
			Message:    fmt.Sprintf("unsupported semantic rule %q", rule),
			FixSafety:  config.FixNone,
			GateStatus: config.GateFail,
		}, nil
	}
}

type semanticConfig struct {
	Rules   []string
	Exclude []string
}

func (a SemanticAdapter) semanticConfig(stepCtx StepContext) (semanticConfig, error) {
	if strings.TrimSpace(a.cfg.Parser) != "" {
		return semanticConfig{Rules: []string{strings.TrimSpace(a.cfg.Parser)}, Exclude: defaultSemanticExcludes()}, nil
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
		merged.Exclude = append(merged.Exclude, loaded.Exclude...)
	}
	if len(merged.Rules) == 0 {
		merged.Rules = []string{"no-direct-os-getenv"}
	}
	merged.Exclude = normalizeSemanticExcludes(append(defaultSemanticExcludes(), merged.Exclude...))
	return merged, nil
}

func defaultSemanticExcludes() []string {
	return []string{".git", ".go-review", "artifacts", "vendor"}
}

func loadSemanticConfig(path string) (semanticConfig, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return semanticConfig{}, nil
	}
	if err != nil {
		return semanticConfig{}, err
	}
	var cfg semanticConfig
	section := ""
	for lineNo, raw := range strings.Split(string(data), "\n") {
		line := strings.TrimSpace(stripSemanticComment(raw))
		if line == "" {
			continue
		}
		key, values, ok := parseSemanticListHeader(line)
		if ok {
			section = key
			switch key {
			case "rules":
				cfg.Rules = append(cfg.Rules, values...)
			case "exclude":
				cfg.Exclude = append(cfg.Exclude, values...)
			default:
				return semanticConfig{}, fmt.Errorf("%s:%d: unsupported semantic config field %q", path, lineNo+1, key)
			}
			continue
		}
		if strings.HasPrefix(line, "- ") {
			value := cleanSemanticValue(strings.TrimSpace(strings.TrimPrefix(line, "- ")))
			if value == "" {
				continue
			}
			switch section {
			case "rules":
				cfg.Rules = append(cfg.Rules, value)
			case "exclude":
				cfg.Exclude = append(cfg.Exclude, value)
			default:
				return semanticConfig{}, fmt.Errorf("%s:%d: list item must be under rules or exclude", path, lineNo+1)
			}
			continue
		}
		return semanticConfig{}, fmt.Errorf("%s:%d: expected rules/exclude list item", path, lineNo+1)
	}
	cfg.Exclude = normalizeSemanticExcludes(cfg.Exclude)
	return cfg, nil
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

func normalizeSemanticExcludes(values []string) []string {
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

func (a SemanticAdapter) runNoDirectOSGetenv(excludes []string, stepCtx StepContext) (Result, error) {
	files, err := goFiles(resolveWorkdir(stepCtx.ProjectRoot, a.cfg.Workdir), excludes)
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
		names[importPath] = struct{}{}
	}
	return names
}

func goFiles(root string, excludes []string) ([]string, error) {
	var files []string
	excludes = normalizeSemanticExcludes(append(defaultSemanticExcludes(), excludes...))
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			if semanticPathExcluded(root, path, entry.Name(), excludes) {
				return filepath.SkipDir
			}
			return nil
		}
		if strings.HasSuffix(entry.Name(), ".go") && !semanticPathExcluded(root, path, entry.Name(), excludes) {
			files = append(files, path)
		}
		return nil
	})
	sort.Strings(files)
	return files, err
}

func semanticPathExcluded(root, path, name string, excludes []string) bool {
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
