package engine

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/v111nce/go-review/internal/config"
)

// LLMReviewAdapter 负责把 C 类 LLM 审阅规则交给外部 Codex 进程。
//
// 它只在用户显式启用 llm.review step 时运行；默认配置保持注释状态，避免普通 check
// 隐式调用模型。执行方式是直接启动 `codex exec` 子进程，不依赖 tmux；go-review 会
// 生成稳定 prompt、等待 Codex 结束，并把 stdout/stderr 写入 artifact 供报告追踪。
type LLMReviewAdapter struct {
	cfg config.Adapter
}

func (a LLMReviewAdapter) Metadata() AdapterMetadata {
	return AdapterMetadata{ID: a.cfg.ID, Type: "llm.review", Capabilities: a.cfg.Capabilities, Version: a.cfg.Version}
}

// Run 直接执行 Codex LLM review。
//
// 这一步的 gate 语义是 Codex 进程是否成功结束。若 Codex 发现并修改问题，调用方仍应按
// latest.llm.md 中的完成标准重新运行 go-review check 或 go test 验证。
func (a LLMReviewAdapter) Run(ctx context.Context, stepCtx StepContext) (Result, error) {
	codexCommand := llmReviewCodexCommand(a.cfg)
	if _, err := exec.LookPath(codexCommand); err != nil {
		return Result{AdapterID: a.cfg.ID, StepID: stepCtx.Step.ID, RuleID: "llm.review.codex", Kind: ResultViolation, Message: fmt.Sprintf("llm.review requires %s in PATH", codexCommand), Suggestion: "安装并登录 Codex CLI 后再启用 llm.review adapter", FixSafety: config.FixNone, GateStatus: config.GateFail}, nil
	}

	rulesPath := llmReviewRulesPath(a.cfg, stepCtx)
	promptPath, prompt, err := writeLLMReviewPrompt(stepCtx, rulesPath)
	if err != nil {
		return Result{AdapterID: a.cfg.ID, StepID: stepCtx.Step.ID, RuleID: "llm.review.prompt", Kind: ResultViolation, Message: err.Error(), FixSafety: config.FixNone, GateStatus: config.GateFail}, nil
	}

	reviewRoot := llmReviewRoot(stepCtx)
	cmd := exec.CommandContext(ctx, codexCommand, "exec", "--cd", reviewRoot, "-")
	cmd.Dir = reviewRoot
	cmd.Env = mergeEnv(os.Environ(), a.cfg.Env)
	cmd.Stdin = strings.NewReader(prompt)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err = cmd.Run()

	status := config.GatePass
	message := "llm.review codex completed"
	if err != nil {
		status = config.GateFail
		message = err.Error()
	}
	processPath, processWriteErr := appendLLMProcessSection(stepCtx, "第一模型执行结果", stepCtx.Step.ID, stdout.String(), stderr.String(), status, message)
	if processWriteErr != nil && status == config.GatePass {
		status = config.GateFail
		message = processWriteErr.Error()
	}
	artifacts := []Artifact{
		{Name: "prompt", Path: promptPath, Content: prompt},
		{Name: "stdout", Content: stdout.String()},
		{Name: "stderr", Content: stderr.String()},
	}
	if processPath != "" {
		artifacts = append(artifacts, Artifact{Name: "process", Path: processPath, Content: stdout.String()})
	}
	if dir := stepCtx.Config.Artifacts.Dir; dir != "" {
		if written, writeErr := writeArtifacts(configRelativePath(stepCtx.ConfigPath, stepCtx.ProjectRoot, dir), stepCtx.Step.ID, artifactsWithoutPresetPath(artifacts)); writeErr == nil {
			artifacts = append(written, artifactsWithPresetPath(artifacts)...)
		}
	}
	return Result{AdapterID: a.cfg.ID, StepID: stepCtx.Step.ID, RuleID: "llm.review", Kind: ResultArtifact, Message: message, FixSafety: a.cfg.FixSafety, GateStatus: status, Artifacts: artifacts}, nil
}

func llmReviewCodexCommand(cfg config.Adapter) string {
	if strings.TrimSpace(cfg.Command) != "" {
		return strings.TrimSpace(cfg.Command)
	}
	return "codex"
}

func llmReviewRulesPath(cfg config.Adapter, stepCtx StepContext) string {
	for i := 0; i < len(cfg.Args); i++ {
		arg := strings.TrimSpace(cfg.Args[i])
		if arg == "--rules" && i+1 < len(cfg.Args) {
			return configRelativePath(stepCtx.ConfigPath, stepCtx.ProjectRoot, cfg.Args[i+1])
		}
		if strings.HasPrefix(arg, "--rules=") {
			return configRelativePath(stepCtx.ConfigPath, stepCtx.ProjectRoot, strings.TrimPrefix(arg, "--rules="))
		}
	}
	return absPath(filepath.Join(filepath.Dir(stepCtx.ConfigPath), "llm-rules.json"))
}

func writeLLMReviewPrompt(stepCtx StepContext, rulesPath string) (string, string, error) {
	reportPath := latestLLMReportPath(stepCtx.ConfigPath)
	promptPath := filepath.Join(configRelativePath(stepCtx.ConfigPath, stepCtx.ProjectRoot, stepCtx.Config.Artifacts.Dir), stepCtx.Step.ID+"-prompt.md")
	if stepCtx.Config.Artifacts.Dir == "" {
		promptPath = filepath.Join(filepath.Dir(stepCtx.ConfigPath), "artifacts", "latest", stepCtx.Step.ID+"-prompt.md")
	}
	content := fmt.Sprintf(`# go-review LLM 审阅任务

你正在审阅一个 Go 项目。请读取下列文件：

- LLM 修复上下文：%s
- LLM 规则文件：%s

要求：

1. 优先修复 latest.llm.md 中确定性工具报告的失败项。
2. 按 llm-rules.json 中 handling=llm-review 的规则审阅当前项目或当前改动。
3. 输出和修改说明必须带 rule_id、文件位置、原因、建议。
4. 不要修改 .go-review/artifacts/ 或 .go-review/reports/ 下的生成产物。
5. 完成后运行合适的 go-review check 或 go test 验证。
`, reportPath, rulesPath)
	if err := os.MkdirAll(filepath.Dir(promptPath), 0o755); err != nil {
		return "", "", err
	}
	if err := os.WriteFile(promptPath, []byte(content), 0o644); err != nil {
		return "", "", err
	}
	return promptPath, content, nil
}

func latestLLMReportPath(configPath string) string {
	dir := filepath.Dir(configPath)
	if filepath.Base(dir) == ".go-review" {
		return absPath(filepath.Join(dir, "reports", "latest.llm.md"))
	}
	return absPath(filepath.Join(dir, ".go-review", "reports", "latest.llm.md"))
}

// llmReviewRoot 返回 Codex 进程的工作目录。
//
// 普通工具 step 会在 defaults.workdir 下执行，方便定位 Go module；但 llm.review 需要读取
// 仓库级 .go-review/reports/latest.llm.md 和 llm-rules.json。若配置文件位于仓库根的
// .go-review/go-review.yaml，则让 Codex 从仓库根启动，避免像 ailx-agent 这类 workdir=api
// 的项目在 api/.go-review 下找不到报告和规则。
func llmReviewRoot(stepCtx StepContext) string {
	configDir := filepath.Dir(absPath(stepCtx.ConfigPath))
	if filepath.Base(configDir) == ".go-review" {
		return filepath.Dir(configDir)
	}
	return configDir
}

func configRelativePath(configPath, projectRoot, value string) string {
	if value == "" {
		return projectRoot
	}
	if filepath.IsAbs(value) {
		return filepath.Clean(value)
	}
	return absPath(filepath.Join(filepath.Dir(configPath), value))
}

func absPath(path string) string {
	abs, err := filepath.Abs(path)
	if err != nil {
		return filepath.Clean(path)
	}
	return filepath.Clean(abs)
}
