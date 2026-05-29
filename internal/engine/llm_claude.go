package engine

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/v111nce/go-review/internal/config"
)

// ClaudeReviewAdapter 负责在 Codex LLM review 之后做可选的第二模型复审。
//
// 它的定位不是替代 latest.llm.md，而是消费 latest.llm.md、llm-rules.json 和 Codex
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

	rulesPath := llmReviewRulesPath(a.cfg, stepCtx)
	promptPath, prompt, err := writeClaudeReviewPrompt(stepCtx, rulesPath)
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
	resultPath, resultWriteErr := appendLLMProcessSection(stepCtx, "第二模型复盘结果", "llm-claude", stdout.String(), stderr.String(), status, message)
	if resultWriteErr != nil && status == config.GatePass {
		status = config.GateFail
		message = resultWriteErr.Error()
	}
	artifacts := []Artifact{
		{Name: "prompt", Path: promptPath, Content: prompt},
		{Name: "stdout", Content: stdout.String()},
		{Name: "stderr", Content: stderr.String()},
	}
	if resultPath != "" {
		artifacts = append(artifacts, Artifact{Name: "review", Path: resultPath, Content: stdout.String()})
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

func writeClaudeReviewPrompt(stepCtx StepContext, rulesPath string) (string, string, error) {
	reportPath := latestLLMReportPath(stepCtx.ConfigPath)
	promptPath := filepath.Join(configRelativePath(stepCtx.ConfigPath, stepCtx.ProjectRoot, stepCtx.Config.Artifacts.Dir), stepCtx.Step.ID+"-prompt.md")
	if stepCtx.Config.Artifacts.Dir == "" {
		promptPath = filepath.Join(filepath.Dir(stepCtx.ConfigPath), "artifacts", "latest", stepCtx.Step.ID+"-prompt.md")
	}
	codexStdout := filepath.Join(configRelativePath(stepCtx.ConfigPath, stepCtx.ProjectRoot, stepCtx.Config.Artifacts.Dir), "llm-review-stdout.txt")
	codexStderr := filepath.Join(configRelativePath(stepCtx.ConfigPath, stepCtx.ProjectRoot, stepCtx.Config.Artifacts.Dir), "llm-review-stderr.txt")
	processReport := latestProcessReportPath(stepCtx)
	content := fmt.Sprintf(`# go-review Claude 复审任务

你正在作为第二模型复审一个 Go 项目的代码质量修复。请读取下列文件：

- 机器生成的 LLM 修复上下文：%s
- LLM 规则文件：%s
- Codex stdout 产物：%s
- Codex stderr 产物：%s

要求：

1. 先判断 Codex 对当前代码质量问题的修复是否完整、是否引入回归或过度修改。
2. 继续按 llm-rules.json 中 handling=llm-review 的规则审阅当前项目或当前改动。
3. 对能安全修复的问题直接修改；对不确定的问题只输出点评，不要盲改。
4. 输出和修改说明必须使用中文，并带 rule_id、文件位置、原因、建议。
5. 不要修改 .go-review/artifacts/ 或 .go-review/reports/ 下的生成产物。
6. 完成后运行合适的 go-review check 或 go test 验证；如果无法运行，说明原因。
7. go-review 会把你的 stdout 追加进统一过程文档：%s。
`, reportPath, rulesPath, codexStdout, codexStderr, processReport)
	if err := os.MkdirAll(filepath.Dir(promptPath), 0o755); err != nil {
		return "", "", err
	}
	if err := os.WriteFile(promptPath, []byte(content), 0o644); err != nil {
		return "", "", err
	}
	return promptPath, content, nil
}

func appendLLMProcessSection(stepCtx StepContext, title, stepID, stdout, stderr string, status config.GateStatus, message string) (string, error) {
	path := latestProcessReportPath(stepCtx)
	if strings.TrimSpace(path) == "" {
		return "", nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", err
	}
	section := llmProcessSectionMarkdown(title, stepID, stdout, stderr, status, message)
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return "", err
	}
	defer file.Close()
	if _, err := file.WriteString(section); err != nil {
		return "", err
	}
	if err := appendLLMProcessRunCopy(stepCtx, section); err != nil {
		return "", err
	}
	return path, nil
}

func appendLLMProcessRunCopy(stepCtx StepContext, section string) error {
	dir := processReportDir(stepCtx)
	if strings.TrimSpace(dir) == "" {
		return nil
	}
	runsDir := filepath.Join(dir, "runs")
	if err := os.MkdirAll(runsDir, 0o755); err != nil {
		return err
	}
	stamp := time.Now().UTC().Format("20060102T150405Z")
	return os.WriteFile(filepath.Join(runsDir, stamp+".process.md"), []byte(section), 0o644)
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

func latestProcessReportPath(stepCtx StepContext) string {
	dir := processReportDir(stepCtx)
	if dir == "" {
		return ""
	}
	return absPath(filepath.Join(dir, "latest.process.md"))
}

func processReportDir(stepCtx StepContext) string {
	if strings.TrimSpace(stepCtx.ReportDir) != "" {
		return stepCtx.ReportDir
	}
	dir := filepath.Dir(stepCtx.ConfigPath)
	if filepath.Base(dir) == ".go-review" {
		return filepath.Join(dir, "reports")
	}
	return filepath.Join(dir, ".go-review", "reports")
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
