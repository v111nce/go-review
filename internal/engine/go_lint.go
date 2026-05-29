package engine

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/v111nce/go-review/internal/config"
)

// GoLintAdapter 负责把 go-review 的 go.lint adapter 语义落到 golangci-lint。
//
// 这里不直接重新实现 gofmt、goimports、govet 等工具能力，而是把它们统一交给
// golangci-lint fmt/run 执行；go-review 只补充编排、默认参数、排除目录、报告归一和
// rule catalog 映射。这样后续新增 linter 时可以优先调整配置，而不是复制外部工具逻辑。
type GoLintAdapter struct {
	cfg config.Adapter
}

func (a GoLintAdapter) Metadata() AdapterMetadata {
	return AdapterMetadata{ID: a.cfg.ID, Type: "go.lint", Capabilities: a.cfg.Capabilities, Version: a.cfg.Version}
}

// Run 执行一次 golangci-lint 检查或安全修复。
//
// 关键行为：
//  1. check 模式下给 fmt 子命令自动追加 --diff，只报告会被修改的文件；
//  2. fix 模式下移除 --diff，让 golangci-lint fmt 真正写回格式化结果；
//  3. run 子命令失败时解析第一条 linter 输出，归一为 file/line/column/message；
//  4. 根据 formatter/linter 名称映射到 rules/go-rules.json 里的稳定 rule_id。
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
	if !isGolangciLintFmt(args) {
		applyGolangciLintNoiseFilter(&result, stepCtx)
	}
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

// applyGolangciLintNoiseFilter 过滤 go-review 默认策略认定的跨项目高概率误报。
//
// 这些过滤项不是通用 golangci-lint 能力替代，而是为了让旧项目里仍带 `--no-config` 的
// go-review.yaml 也能获得同一套低噪音默认行为：go-zero 的 optional 配置标签不作为失败项，
// 测试中启动本地子进程的 G204 不作为默认安全门禁。
func applyGolangciLintNoiseFilter(result *Result, stepCtx StepContext) {
	changed := false
	for i, artifact := range result.Artifacts {
		filtered := filterGolangciLintNoiseLines(artifact.Content)
		if filtered != artifact.Content {
			result.Artifacts[i].Content = filtered
			changed = true
		}
	}
	if !changed {
		return
	}
	if dir := stepCtx.Config.Artifacts.Dir; dir != "" {
		if written, err := writeArtifacts(resolveWorkdir(stepCtx.ProjectRoot, dir), stepCtx.Step.ID, result.Artifacts); err == nil {
			result.Artifacts = written
		}
	}
	if strings.TrimSpace(combinedArtifactContent(result.Artifacts)) == "" {
		result.GateStatus = config.GatePass
		result.Message = "golangci-lint run clean"
	}
}

func filterGolangciLintNoiseLines(content string) string {
	if strings.TrimSpace(content) == "" {
		return content
	}
	var kept []string
	for _, line := range strings.Split(content, "\n") {
		if isNoisyGolangciLintLine(line) {
			continue
		}
		kept = append(kept, line)
	}
	return strings.Join(kept, "\n")
}

func isNoisyGolangciLintLine(line string) bool {
	return strings.Contains(line, "SA5008: invalid appearance of unknown `optional` tag option") ||
		(strings.Contains(line, "_test.go:") && strings.Contains(line, "G204: Subprocess launched with variable"))
}

func combinedArtifactContent(artifacts []Artifact) string {
	var b strings.Builder
	for _, artifact := range artifacts {
		b.WriteString(artifact.Content)
	}
	return b.String()
}

// configRelativeGolangciArgs 把 --config 指向的相对路径解析成相对当前执行 workdir 可用的路径。
//
// go-review 的配置文件通常在仓库根 .go-review/go-review.yaml，而 adapter 实际执行目录可能是
// defaults.workdir（例如 api/）。若直接把 `.go-review/golangci.yml` 传给 golangci-lint，它会在
// api/.go-review 下查找并失败；这里统一按 go-review.yaml 所在目录解析后转成 workdir 相对路径。
func configRelativeGolangciArgs(args []string, stepCtx StepContext) []string {
	out := append([]string{}, args...)
	workdir := resolveWorkdir(stepCtx.ProjectRoot, stepCtx.Adapter.Workdir)
	for i := 0; i < len(out); i++ {
		arg := out[i]
		if arg == "--config" || arg == "-c" {
			if i+1 < len(out) {
				out[i+1] = workdirRelativeConfigPath(out[i+1], stepCtx.ConfigPath, workdir)
				i++
			}
			continue
		}
		for _, prefix := range []string{"--config=", "-c="} {
			if strings.HasPrefix(arg, prefix) {
				out[i] = prefix + workdirRelativeConfigPath(strings.TrimPrefix(arg, prefix), stepCtx.ConfigPath, workdir)
				break
			}
		}
	}
	return out
}

func workdirRelativeConfigPath(value, goReviewConfigPath, workdir string) string {
	value = strings.TrimSpace(value)
	if value == "" || filepath.IsAbs(value) {
		return value
	}
	configDir := filepath.Dir(goReviewConfigPath)
	if strings.HasPrefix(filepath.ToSlash(value), ".go-review/") && filepath.Base(configDir) == ".go-review" {
		configDir = filepath.Dir(configDir)
	}
	absConfig := filepath.Join(configDir, value)
	rel, err := filepath.Rel(workdir, absConfig)
	if err != nil {
		return absPath(absConfig)
	}
	return filepath.ToSlash(rel)
}

// formatDiffFile 从 golangci-lint fmt --diff 的 artifact 中提取首个受影响文件。
//
// format-check 只有 diff 文本，没有标准的 file:line 输出；这里兼容 git diff、普通
// diff 以及 ---/+++ 文件头三种形式，尽量给报告补上“位置”字段，方便用户定位。
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

// applyGolangciLintFinding 把 golangci-lint run 的第一条文本诊断归一到 Result。
//
// 当前报告面只展示一个阻断点，因此取 artifact 中第一条非空诊断；完整原始输出仍保留
// 在 artifact 文件里，避免丢失后续 linter 的细节。
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

// firstNonEmptyArtifactLine 返回 artifact 中第一条非空输出。
// golangci-lint run 的失败输出通常一行就是一个 finding，可直接进入解析流程。
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

// parseGolangciLintLine 解析 golangci-lint 常见输出：file.go:line:column: message (linter)。
//
// 如果输出不符合这个格式，则保留原始文本作为 message，避免因为解析失败吞掉错误。
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

// parsePositiveInt 用极小依赖解析正整数。
// 这里刻意不返回 strconv 的详细错误，因为调用方只需要知道字段是否能作为位置使用。
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

// golangciLinterRuleID 把 golangci-lint 的 linter 名称映射为本项目 catalog rule_id。
//
// rule_id 是报告和 rules/go-rules.json 之间的稳定关联键；外部 linter 名称只是实现细节。
// 多个 linter 可能覆盖同一规范主题，例如 errcheck/govet 都可能落到错误处理规则。
func golangciLinterRuleID(linter string) string {
	switch strings.TrimSpace(linter) {
	case "bodyclose":
		return "uber.guideline.defer-cleanup"
	case "errcheck":
		return "go.official.handle-errors"
	case "errname":
		return "uber.guideline.error-naming"
	case "errorlint":
		return "uber.guideline.error-wrapping"
	case "forcetypeassert":
		return "uber.guideline.type-assertion-ok"
	case "gochecknoglobals":
		return "uber.guideline.mutable-globals"
	case "gochecknoinits":
		return "uber.guideline.no-init"
	case "godot":
		return "go.official.comment-sentences"
	case "gosec":
		return "go.official.crypto-rand"
	case "govet":
		return "go.official.handle-errors"
	case "importas":
		return "google.imports.renaming"
	case "makezero":
		return "uber.style.maps"
	case "nakedret":
		return "go.official.naked-returns"
	case "nonamedreturns":
		return "go.official.named-results"
	case "paralleltest":
		return "uber.pattern.parallel-tests"
	case "perfsprint":
		return "uber.perf.strconv"
	case "prealloc":
		return "uber.style.maps"
	case "predeclared":
		return "uber.guideline.builtin-names"
	case "revive":
		return "go.official.identifier-style"
	case "staticcheck":
		return "google.bp.zero-values"
	case "testifylint":
		return "go.test.no-assert-libraries"
	case "thelper":
		return "go.test.mark-helpers"
	case "varnamelen":
		return "google.naming.single-letter-vars"
	default:
		return ""
	}
}

// cleanDiffPath 清理 diff 或 linter 输出中的路径前缀，并尽量转成项目相对路径。
func cleanDiffPath(value, projectRoot string) string {
	value = strings.TrimPrefix(value, "a/")
	value = strings.TrimPrefix(value, "b/")
	value = filepath.Clean(value)
	if filepath.IsAbs(value) {
		return relPath(projectRoot, value)
	}
	return filepath.ToSlash(value)
}

// goLintRuleID 根据 golangci-lint 参数推断本次 adapter 的默认 rule_id。
//
// 对 run：只有启用单个 linter 时才精确映射；如果一次启用多个 linter，就回退到
// go.lint，避免把组合检查错误归因到某一条 catalog rule。
// 对 fmt：gofmt/gofumpt 映射代码格式规则，goimports/gci 映射 import 分组规则。
func goLintRuleID(args []string) string {
	if !isGolangciLintFmt(args) {
		linters := enabledGolangciLinters(args)
		if len(linters) == 1 {
			if mapped := golangciLinterRuleID(linters[0]); mapped != "" {
				return mapped
			}
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

// enabledGolangciFormatters 提取 golangci-lint fmt 的 --enable/-E formatter 名称。
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

// enabledGolangciLinters 提取 golangci-lint run 的显式 linter 名称。
//
// 同时支持 --enable x、--enable=x、-E=x、--enable-only x、--enable-only=x；
// --enable-only 里经常写逗号列表，所以后续会继续拆成多个名称。
func enabledGolangciLinters(args []string) []string {
	var linters []string
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--enable" || arg == "-E" || arg == "--enable-only":
			if i+1 < len(args) {
				linters = appendGolangciNames(linters, args[i+1])
				i++
			}
		case strings.HasPrefix(arg, "--enable="):
			linters = appendGolangciNames(linters, strings.TrimPrefix(arg, "--enable="))
		case strings.HasPrefix(arg, "-E="):
			linters = appendGolangciNames(linters, strings.TrimPrefix(arg, "-E="))
		case strings.HasPrefix(arg, "--enable-only="):
			linters = appendGolangciNames(linters, strings.TrimPrefix(arg, "--enable-only="))
		}
	}
	return linters
}

// appendGolangciNames 追加逗号分隔的 formatter/linter 名称。
// 例如 --enable-only=errcheck,govet 会被拆成 errcheck 和 govet 两个独立项。
func appendGolangciNames(out []string, value string) []string {
	for _, part := range strings.Split(value, ",") {
		if part = strings.TrimSpace(part); part != "" {
			out = append(out, part)
		}
	}
	return out
}

// hasFormatter 判断 formatter 列表中是否启用了指定名称。
func hasFormatter(formatters []string, want string) bool {
	for _, formatter := range formatters {
		if formatter == want {
			return true
		}
	}
	return false
}

// golangciLintCommand 返回用户配置的 golangci-lint 路径；未配置时使用 PATH 中的默认命令。
func golangciLintCommand(configured string) string {
	if strings.TrimSpace(configured) != "" {
		return configured
	}
	return "golangci-lint"
}

// golangciLintArgs 生成最终传给 golangci-lint 的参数。
//
// 默认没有 args 时执行 `golangci-lint fmt --no-config --enable gofmt`。如果是 check
// 模式，自动补 --diff；如果是 fix 模式，移除 --diff。fmt 子命令在未显式传路径时
// 会自动展开项目内 Go 文件，并应用默认/用户排除目录，避免扫到 vendor/testdata 等区域。
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
	if !isGolangciLintFmt(args) {
		args = configRelativeGolangciArgs(args, stepCtx)
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

// isGolangciLintFmt 判断参数是否表示 golangci-lint fmt 子命令。
// 它会跳过前置 flag；遇到第一个非 flag 参数不是 fmt 时，就按 run/其他命令处理。
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

// hasPathArg 判断用户是否已经在参数中显式指定了检查路径。
//
// fmt 子命令只有在没有路径参数时才由 go-review 自动补文件列表；这里必须识别并跳过
// --enable/--enable-only/--disable 这类带值 flag，否则会把 linter 名误判成文件路径。
func hasPathArg(args []string) bool {
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "fmt" || arg == "--diff" || arg == "--no-config" {
			continue
		}
		if arg == "--enable" || arg == "-E" || arg == "--enable-only" || arg == "--disable" || arg == "-D" {
			i++
			continue
		}
		if strings.HasPrefix(arg, "--enable=") || strings.HasPrefix(arg, "--enable-only=") || strings.HasPrefix(arg, "--disable=") {
			continue
		}
		if strings.HasPrefix(arg, "-") {
			continue
		}
		return true
	}
	return false
}

// hasArg 判断完整参数列表里是否存在指定 flag。
func hasArg(args []string, want string) bool {
	for _, arg := range args {
		if arg == want {
			return true
		}
	}
	return false
}

// removeArg 删除指定 flag，主要用于 fix 模式下移除 check 模式自动追加的 --diff。
func removeArg(args []string, remove string) []string {
	out := args[:0]
	for _, arg := range args {
		if arg != remove {
			out = append(out, arg)
		}
	}
	return out
}

// defaultProjectExcludes 返回工具层默认排除目录。
//
// 这些排除不写入用户生成的 go-review.yaml，避免让用户误以为是自己配置的；但运行时
// 会默认生效，防止格式化/语义检查进入 .git、.go-review、vendor、testdata 等非源码治理面。
func defaultProjectExcludes() []string {
	return []string{".git", ".go-review", "artifacts", "vendor", "testdata"}
}

// projectExcludes 合并工具默认排除目录和用户在配置中追加的 exclude。
func projectExcludes(cfg *config.Config) []string {
	if cfg == nil {
		return defaultProjectExcludes()
	}
	return append(defaultProjectExcludes(), cfg.Exclude...)
}

// normalizeProjectExcludes 统一排除目录写法，去掉首尾斜杠并去重。
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

// goFiles 收集指定根目录下未被排除的 Go 文件，并保持排序稳定。
// 稳定排序能让报告和测试输出可重复，避免不同文件系统遍历顺序造成噪音。
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

// projectPathExcluded 判断路径是否命中排除规则。
// 规则既匹配目录名，也匹配项目相对路径前缀，例如 testdata 和 internal/testdata 都会被跳过。
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

// relPath 尽量把绝对路径转成项目相对路径；失败时保留原路径以免丢失定位信息。
func relPath(root, file string) string {
	if rel, err := filepath.Rel(root, file); err == nil && !strings.HasPrefix(rel, "..") {
		return rel
	}
	return file
}
