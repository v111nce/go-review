package engine

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/v111nce/go-review/internal/config"
)

type GoLintAdapter struct {
	cfg config.Adapter
}

func (a GoLintAdapter) Metadata() AdapterMetadata {
	return AdapterMetadata{ID: a.cfg.ID, Type: "go.lint", Capabilities: a.cfg.Capabilities, Version: a.cfg.Version}
}

func (a GoLintAdapter) Run(ctx context.Context, stepCtx StepContext) (Result, error) {
	fixMode := stepCtx.Command == CommandFix && stepCtx.Step.AllowFix && a.cfg.FixSafety == config.FixSafe
	args, err := golangciLintArgs(a.cfg.Args, fixMode, stepCtx)
	if err != nil {
		return Result{AdapterID: a.cfg.ID, StepID: stepCtx.Step.ID, RuleID: "go.lint", Kind: ResultViolation, Message: err.Error(), FixSafety: a.cfg.FixSafety, GateStatus: config.GateFail}, nil
	}
	if len(args) == 0 {
		return Result{AdapterID: a.cfg.ID, StepID: stepCtx.Step.ID, RuleID: "go.lint.format", Kind: ResultArtifact, Message: "golangci-lint fmt skipped; no non-excluded Go files", FixSafety: a.cfg.FixSafety, GateStatus: config.GatePass}, nil
	}
	cmdCfg := a.cfg
	cmdCfg.Command = golangciLintCommand(cmdCfg.Command)
	cmdCfg.Args = args
	cmdCfg.Type = "cmd"
	result, err := CommandAdapter{cfg: cmdCfg}.Run(ctx, stepCtx)
	result.RuleID = "go.lint.format"
	result.FixSafety = a.cfg.FixSafety
	result.FixAvailable = a.cfg.FixSafety == config.FixSafe
	result.Kind = ResultArtifact
	if !fixMode {
		if result.GateStatus == config.GateFail {
			result.Kind = ResultViolation
			result.Message = "golangci-lint fmt would change files"
			result.File = formatDiffFile(result.Artifacts, stepCtx.ProjectRoot)
		} else {
			result.Message = "golangci-lint fmt clean"
		}
		return result, err
	}
	if result.GateStatus == config.GatePass {
		result.Message = "golangci-lint fmt applied"
		result.FixApplied = true
	}
	return result, err
}

func formatDiffFile(artifacts []Artifact, projectRoot string) string {
	for _, artifact := range artifacts {
		for _, line := range strings.Split(artifact.Content, "\n") {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "diff --git ") {
				parts := strings.Fields(line)
				if len(parts) >= 4 {
					return cleanDiffPath(parts[2], projectRoot)
				}
			}
			if strings.HasPrefix(line, "diff -- ") {
				parts := strings.Fields(line)
				if len(parts) >= 3 {
					return cleanDiffPath(parts[2], projectRoot)
				}
			}
			if strings.HasPrefix(line, "--- ") || strings.HasPrefix(line, "+++ ") {
				parts := strings.Fields(line)
				if len(parts) >= 2 && parts[1] != "/dev/null" {
					return cleanDiffPath(parts[1], projectRoot)
				}
			}
		}
	}
	return ""
}

func cleanDiffPath(value, projectRoot string) string {
	value = strings.TrimPrefix(value, "a/")
	value = strings.TrimPrefix(value, "b/")
	value = filepath.Clean(value)
	if filepath.IsAbs(value) {
		return relPath(projectRoot, value)
	}
	return filepath.ToSlash(value)
}

func golangciLintCommand(configured string) string {
	if strings.TrimSpace(configured) != "" {
		return configured
	}
	return "golangci-lint"
}

func golangciLintArgs(configured []string, fixMode bool, stepCtx StepContext) ([]string, error) {
	args := append([]string{}, configured...)
	if len(args) == 0 {
		args = []string{"fmt", "--no-config", "--enable", "gofmt"}
	}
	if fixMode {
		args = removeArg(args, "--diff")
	} else if isGolangciLintFmt(args) && !hasArg(args, "--diff") {
		args = append(args, "--diff")
	}
	if !isGolangciLintFmt(args) || hasPathArg(args) {
		return args, nil
	}
	files, err := goFiles(resolveWorkdir(stepCtx.ProjectRoot, stepCtx.Adapter.Workdir), projectExcludes(stepCtx.Config))
	if err != nil {
		return nil, err
	}
	if len(files) == 0 {
		return nil, nil
	}
	workdir := resolveWorkdir(stepCtx.ProjectRoot, stepCtx.Adapter.Workdir)
	for _, file := range files {
		if rel, err := filepath.Rel(workdir, file); err == nil && !strings.HasPrefix(rel, "..") {
			args = append(args, filepath.ToSlash(rel))
		} else {
			args = append(args, file)
		}
	}
	return args, nil
}

func isGolangciLintFmt(args []string) bool {
	for _, arg := range args {
		if arg == "fmt" {
			return true
		}
		if strings.HasPrefix(arg, "-") {
			continue
		}
		return false
	}
	return false
}

func hasPathArg(args []string) bool {
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "fmt" || arg == "--diff" || arg == "--no-config" {
			continue
		}
		if arg == "--enable" || arg == "-E" || arg == "--disable" || arg == "-D" {
			i++
			continue
		}
		if strings.HasPrefix(arg, "--enable=") || strings.HasPrefix(arg, "--disable=") {
			continue
		}
		if strings.HasPrefix(arg, "-") {
			continue
		}
		return true
	}
	return false
}

func hasArg(args []string, want string) bool {
	for _, arg := range args {
		if arg == want {
			return true
		}
	}
	return false
}

func removeArg(args []string, remove string) []string {
	out := args[:0]
	for _, arg := range args {
		if arg != remove {
			out = append(out, arg)
		}
	}
	return out
}

func defaultProjectExcludes() []string {
	return []string{".git", ".go-review", "artifacts", "vendor", "testdata"}
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

func goFiles(root string, excludes []string) ([]string, error) {
	var files []string
	excludes = normalizeProjectExcludes(excludes)
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
	for _, exclude := range normalizeProjectExcludes(excludes) {
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
