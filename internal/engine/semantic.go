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
	rule := strings.TrimSpace(a.cfg.Parser)
	if rule == "" || rule == "no-direct-os-getenv" {
		return a.runNoDirectOSGetenv(stepCtx)
	}
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
		ast.Inspect(parsed, func(node ast.Node) bool {
			call, ok := node.(*ast.CallExpr)
			if !ok {
				return true
			}
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok || sel.Sel == nil || sel.Sel.Name != "Getenv" {
				return true
			}
			obj := info.Uses[sel.Sel]
			if obj == nil || obj.Pkg() == nil || obj.Pkg().Path() != "os" || obj.Name() != "Getenv" {
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
