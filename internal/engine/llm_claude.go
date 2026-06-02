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

// ClaudeReviewAdapter 负责在 Codex LLM review 之后做可选的第二模型复审。
//
// 它的定位不是替代 latest.md，而是消费 latest.md、rules/llm-default.json、rules/llm-custom.json 和 Codex
// 产物，对当前代码质量修复做“独立点评 + 必要修复”。默认配置保持 disabled，只有用户
// 已安装并登录 Claude CLI 且显式开启 llm-claude step 时才运行。
type ClaudeReviewAdapter struct {
	cfg config.Adapter
}

func (a ClaudeReviewAdapter) Metadata() AdapterMetadata {
	return AdapterMetadata{ID: a.cfg.ID, Type: "llm.claude", Capabilities: a.cfg.Capabilities, Version: a.cfg.Version}
}

// Run 直接执行 Claude CLI 的非交互 print 模式。
//
// 默认命令形态是：claude -p --permission-mode acceptEdits <prompt>。
// 如果用户在 adapter.args 中显式传入参数，则完全使用用户参数，并把 prompt 追加为最后一个
// 参数；这样既保留默认可编辑行为，也允许团队改成只读点评、指定模型或限制工具。
func (a ClaudeReviewAdapter) Run(ctx context.Context, stepCtx StepContext) (Result, error) {
	claudeCommand := llmClaudeCommand(a.cfg)
	if _, err := exec.LookPath(claudeCommand); err != nil {
		return Result{AdapterID: a.cfg.ID, StepID: stepCtx.Step.ID, RuleID: "llm.claude.cli", Kind: ResultViolation, Message: fmt.Sprintf("llm.claude requires %s in PATH", claudeCommand), Suggestion: "安装并登录 Claude Code CLI 后再启用 llm.claude adapter", FixSafety: config.FixNone, GateStatus: config.GateFail}, nil
	}

	rulesPaths := defaultLLMRulePaths(stepCtx.ConfigPath)
	promptPath, prompt, err := writeClaudeReviewPrompt(stepCtx, rulesPaths)
	if err != nil {
		return Result{AdapterID: a.cfg.ID, StepID: stepCtx.Step.ID, RuleID: "llm.claude.prompt", Kind: ResultViolation, Message: err.Error(), FixSafety: config.FixNone, GateStatus: config.GateFail}, nil
	}

	reviewRoot := llmReviewRoot(stepCtx)
	args := llmClaudeArgs(a.cfg, prompt)
	cmd := exec.CommandContext(ctx, claudeCommand, args...)
	cmd.Dir = reviewRoot
	cmd.Env = mergeEnv(os.Environ(), a.cfg.Env)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err = cmd.Run()

	status := config.GatePass
	message := "llm.claude completed"
	if err != nil {
		status = config.GateFail
		message = err.Error()
	}
	artifacts := []Artifact{
		{Name: "prompt", Path: promptPath, Content: prompt},
		{Name: "stdout", Content: stdout.String()},
		{Name: "stderr", Content: stderr.String()},
		{Name: "review", Content: llmProcessSectionMarkdown("第二模型复盘结果", stepCtx.Step.ID, stdout.String(), stderr.String(), status, message)},
	}
	if dir := stepCtx.Config.Artifacts.Dir; dir != "" {
		if written, writeErr := writeArtifacts(configRelativePath(stepCtx.ConfigPath, stepCtx.ProjectRoot, dir), stepCtx.Step.ID, artifactsWithoutPresetPath(artifacts)); writeErr == nil {
			artifacts = append(written, artifactsWithPresetPath(artifacts)...)
		}
	}
	return Result{AdapterID: a.cfg.ID, StepID: stepCtx.Step.ID, RuleID: "llm.claude", Kind: ResultArtifact, Message: message, FixSafety: a.cfg.FixSafety, GateStatus: status, Artifacts: artifacts}, nil
}

func llmClaudeCommand(cfg config.Adapter) string {
	if strings.TrimSpace(cfg.Command) != "" {
		return strings.TrimSpace(cfg.Command)
	}
	return "claude"
}

func llmClaudeArgs(cfg config.Adapter, prompt string) []string {
	if len(cfg.Args) > 0 {
		args := append([]string{}, cfg.Args...)
		return append(args, prompt)
	}
	return []string{"-p", "--permission-mode", "acceptEdits", prompt}
}

func writeClaudeReviewPrompt(stepCtx StepContext, rulesPaths []string) (string, string, error) {
	reportPath := latestHumanReportPath(stepCtx.ConfigPath)
	promptPath := filepath.Join(configRelativePath(stepCtx.ConfigPath, stepCtx.ProjectRoot, stepCtx.Config.Artifacts.Dir), stepCtx.Step.ID+"-prompt.md")
	if stepCtx.Config.Artifacts.Dir == "" {
		promptPath = filepath.Join(filepath.Dir(stepCtx.ConfigPath), "artifacts", "latest", stepCtx.Step.ID+"-prompt.md")
	}
	codexStdout := filepath.Join(configRelativePath(stepCtx.ConfigPath, stepCtx.ProjectRoot, stepCtx.Config.Artifacts.Dir), "llm-review-stdout.txt")
	codexStderr := filepath.Join(configRelativePath(stepCtx.ConfigPath, stepCtx.ProjectRoot, stepCtx.Config.Artifacts.Dir), "llm-review-stderr.txt")
	content := fmt.Sprintf(`# go-review Claude 复审任务

你正在作为第二模型复审一个 Go 项目的代码质量修复。请读取下列文件：

- 最新 review 报告：%s
- LLM 默认规则文件：%s
- LLM 自定义规则文件：%s
- Codex stdout 产物：%s
- Codex stderr 产物：%s

要求：

1. 先判断 Codex 对当前代码质量问题的修复是否完整、是否引入回归或过度修改。
2. 继续按 rules/llm-default.json 和 rules/llm-custom.json 中 handling=llm-review 的规则审阅当前项目或当前改动。
3. 对能安全修复的问题直接修改；对不确定的问题只输出点评，不要盲改。
4. 输出和修改说明必须使用中文，并带 rule_id、文件位置、原因、建议。
5. 不要修改 .go-review/artifacts/ 或 .go-review/reports/ 下的生成产物。
6. 完成后运行合适的 go-review check 或 go test 验证；如果无法运行，说明原因。
`, reportPath, firstLLMRulePath(rulesPaths), secondLLMRulePath(rulesPaths), codexStdout, codexStderr)
	if err := os.MkdirAll(filepath.Dir(promptPath), 0o755); err != nil {
		return "", "", err
	}
	if err := os.WriteFile(promptPath, []byte(content), 0o644); err != nil {
		return "", "", err
	}
	return promptPath, content, nil
}

func llmProcessSectionMarkdown(title, stepID, stdout, stderr string, status config.GateStatus, message string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "\n\n## %s：%s\n\n", title, stepID)
	fmt.Fprintf(&b, "- status: `%s`\n", status)
	fmt.Fprintf(&b, "- message: %s\n\n", message)
	fmt.Fprintf(&b, "### 输出\n\n")
	if strings.TrimSpace(stdout) == "" {
		fmt.Fprintf(&b, "（无 stdout 输出）\n")
	} else {
		b.WriteString(stdout)
		if !strings.HasSuffix(stdout, "\n") {
			b.WriteString("\n")
		}
	}
	if strings.TrimSpace(stderr) != "" {
		fmt.Fprintf(&b, "\n### stderr\n\n```text\n%s\n```\n", strings.TrimSpace(stderr))
	}
	return b.String()
}

func artifactsWithoutPresetPath(artifacts []Artifact) []Artifact {
	out := make([]Artifact, 0, len(artifacts))
	for _, artifact := range artifacts {
		if artifact.Path == "" {
			out = append(out, artifact)
		}
	}
	return out
}

func artifactsWithPresetPath(artifacts []Artifact) []Artifact {
	out := make([]Artifact, 0, len(artifacts))
	for _, artifact := range artifacts {
		if artifact.Path != "" {
			out = append(out, artifact)
		}
	}
	return out
}
