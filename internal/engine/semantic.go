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
	rules, err := a.rules(stepCtx)
	if err != nil {
		return Result{AdapterID: a.cfg.ID, StepID: stepCtx.Step.ID, RuleID: "semantic.config", Kind: ResultViolation, Message: err.Error(), FixSafety: config.FixNone, GateStatus: config.GateFail}, nil
	}
	for _, rule := range rules {
		result, err := a.runRule(rule, stepCtx)
		if err != nil || result.GateStatus == config.GateFail {
			return result, err
		}
	}
	return Result{
		AdapterID:  a.cfg.ID,
		StepID:     stepCtx.Step.ID,
		RuleID:     "semantic.rules",
		Kind:       ResultArtifact,
		Message:    fmt.Sprintf("semantic rules passed (%d)", len(rules)),
		FixSafety:  a.cfg.FixSafety,
		GateStatus: config.GatePass,
	}, nil
}

func (a SemanticAdapter) runRule(rule string, stepCtx StepContext) (Result, error) {
	switch rule {
	case "", "no-direct-os-getenv":
		return a.runNoDirectOSGetenv(stepCtx)
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

func (a SemanticAdapter) rules(stepCtx StepContext) ([]string, error) {
	if strings.TrimSpace(a.cfg.Parser) != "" {
		return []string{strings.TrimSpace(a.cfg.Parser)}, nil
	}
	paths := []string{
		filepath.Join(filepath.Dir(stepCtx.ConfigPath), "semantic", "default.yaml"),
		filepath.Join(filepath.Dir(stepCtx.ConfigPath), "semantic", "custom.yaml"),
	}
	var rules []string
	for _, path := range paths {
		loaded, err := loadSemanticRules(path)
		if err != nil {
			return nil, err
		}
		rules = append(rules, loaded...)
	}
	if len(rules) == 0 {
		return []string{"no-direct-os-getenv"}, nil
	}
	return rules, nil
}

func loadSemanticRules(path string) ([]string, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var rules []string
	for lineNo, raw := range strings.Split(string(data), "\n") {
		line := strings.TrimSpace(stripSemanticComment(raw))
		if line == "" || line == "rules:" {
			continue
		}
		if strings.HasPrefix(line, "- ") {
			rule := strings.TrimSpace(strings.TrimPrefix(line, "- "))
			rule = strings.Trim(rule, "\"'")
			if rule != "" {
				rules = append(rules, rule)
			}
			continue
		}
		return nil, fmt.Errorf("%s:%d: expected rules list item", path, lineNo+1)
	}
	return rules, nil
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

func (a SemanticAdapter) runNoDirectOSGetenv(stepCtx StepContext) (Result, error) {
	files, err := goFiles(resolveWorkdir(stepCtx.ProjectRoot, a.cfg.Workdir))
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

func goFiles(root string) ([]string, error) {
	var files []string
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			switch entry.Name() {
			case ".git", "artifacts", "vendor":
				return filepath.SkipDir
			}
			return nil
		}
		if strings.HasSuffix(entry.Name(), ".go") {
			files = append(files, path)
		}
		return nil
	})
	sort.Strings(files)
	return files, err
}

func relPath(root, file string) string {
	if rel, err := filepath.Rel(root, file); err == nil && !strings.HasPrefix(rel, "..") {
		return rel
	}
	return file
}
