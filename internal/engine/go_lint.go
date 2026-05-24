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
	ruleID := goLintRuleID(args)
	if len(args) == 0 {
		return Result{AdapterID: a.cfg.ID, StepID: stepCtx.Step.ID, RuleID: ruleID, Kind: ResultArtifact, Message: "golangci-lint fmt skipped; no non-excluded Go files", FixSafety: a.cfg.FixSafety, GateStatus: config.GatePass}, nil
	}
	cmdCfg := a.cfg
	cmdCfg.Command = golangciLintCommand(cmdCfg.Command)
	cmdCfg.Args = args
	cmdCfg.Type = "cmd"
	result, err := CommandAdapter{cfg: cmdCfg}.Run(ctx, stepCtx)
	result.RuleID = ruleID
	result.FixSafety = a.cfg.FixSafety
	result.FixAvailable = a.cfg.FixSafety == config.FixSafe && isGolangciLintFmt(args)
	result.Kind = ResultArtifact
	if !fixMode {
		if result.GateStatus == config.GateFail {
			result.Kind = ResultViolation
			if isGolangciLintFmt(args) {
				result.Message = "golangci-lint fmt would change files"
				result.File = formatDiffFile(result.Artifacts, stepCtx.ProjectRoot)
			} else {
				applyGolangciLintFinding(&result, stepCtx.ProjectRoot)
			}
		} else if isGolangciLintFmt(args) {
			result.Message = "golangci-lint fmt clean"
		} else {
			result.Message = "golangci-lint run clean"
		}
		return result, err
	}
	if result.GateStatus == config.GatePass {
		if isGolangciLintFmt(args) {
			result.Message = "golangci-lint fmt applied"
			result.FixApplied = true
		} else {
			result.Message = "golangci-lint run clean"
		}
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

func applyGolangciLintFinding(result *Result, projectRoot string) {
	line := firstNonEmptyArtifactLine(result.Artifacts)
	if line == "" {
		if result.Message == "" {
			result.Message = "golangci-lint run failed"
		}
		return
	}
	file, lineNo, col, message, linter := parseGolangciLintLine(line)
	if file != "" {
		result.File = cleanDiffPath(file, projectRoot)
	}
	result.Line = lineNo
	result.Column = col
	if message != "" {
		result.Message = message
	}
	if mapped := golangciLinterRuleID(linter); mapped != "" {
		result.RuleID = mapped
	}
}

func firstNonEmptyArtifactLine(artifacts []Artifact) string {
	for _, artifact := range artifacts {
		for _, line := range strings.Split(artifact.Content, "\n") {
			line = strings.TrimSpace(line)
			if line != "" {
				return line
			}
		}
	}
	return ""
}

func parseGolangciLintLine(line string) (file string, lineNo int, column int, message string, linter string) {
	parts := strings.SplitN(line, ":", 4)
	if len(parts) < 4 {
		return "", 0, 0, line, ""
	}
	parsedLine, lineOK := parsePositiveInt(strings.TrimSpace(parts[1]))
	parsedColumn, columnOK := parsePositiveInt(strings.TrimSpace(parts[2]))
	if !lineOK || !columnOK {
		return "", 0, 0, line, ""
	}
	message = strings.TrimSpace(parts[3])
	if open := strings.LastIndex(message, "("); open >= 0 && strings.HasSuffix(message, ")") {
		linter = strings.TrimSpace(strings.TrimSuffix(message[open+1:], ")"))
		message = strings.TrimSpace(message[:open])
	}
	return strings.TrimSpace(parts[0]), parsedLine, parsedColumn, message, linter
}

func parsePositiveInt(value string) (int, bool) {
	if value == "" {
		return 0, false
	}
	n := 0
	for _, r := range value {
		if r < '0' || r > '9' {
			return 0, false
		}
		n = n*10 + int(r-'0')
	}
	return n, n > 0
}

func golangciLinterRuleID(linter string) string {
	switch strings.TrimSpace(linter) {
	case "errcheck":
		return "go.official.handle-errors"
	default:
		return ""
	}
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

func goLintRuleID(args []string) string {
	if !isGolangciLintFmt(args) {
		linters := enabledGolangciLinters(args)
		if hasFormatter(linters, "errcheck") {
			return "go.official.handle-errors"
		}
		return "go.lint"
	}
	formatters := enabledGolangciFormatters(args)
	if len(formatters) == 0 {
		return "go.official.gofmt"
	}
	if hasFormatter(formatters, "gofmt") || hasFormatter(formatters, "gofumpt") {
		return "go.official.gofmt"
	}
	if hasFormatter(formatters, "goimports") || hasFormatter(formatters, "gci") {
		return "go.official.imports"
	}
	return "go.lint.format"
}

func enabledGolangciFormatters(args []string) []string {
	var formatters []string
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--enable" || arg == "-E":
			if i+1 < len(args) {
				formatters = append(formatters, strings.TrimSpace(args[i+1]))
				i++
			}
		case strings.HasPrefix(arg, "--enable="):
			formatters = append(formatters, strings.TrimSpace(strings.TrimPrefix(arg, "--enable=")))
		case strings.HasPrefix(arg, "-E="):
			formatters = append(formatters, strings.TrimSpace(strings.TrimPrefix(arg, "-E=")))
		}
	}
	return formatters
}

func enabledGolangciLinters(args []string) []string {
	var linters []string
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--enable" || arg == "-E":
			if i+1 < len(args) {
				linters = append(linters, strings.TrimSpace(args[i+1]))
				i++
			}
		case strings.HasPrefix(arg, "--enable="):
			linters = append(linters, strings.TrimSpace(strings.TrimPrefix(arg, "--enable=")))
		case strings.HasPrefix(arg, "-E="):
			linters = append(linters, strings.TrimSpace(strings.TrimPrefix(arg, "-E=")))
		}
	}
	return linters
}

func hasFormatter(formatters []string, want string) bool {
	for _, formatter := range formatters {
		if formatter == want {
			return true
		}
	}
	return false
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
