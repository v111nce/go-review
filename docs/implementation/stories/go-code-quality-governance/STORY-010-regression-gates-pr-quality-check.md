# STORY-010 Regression Gates PR Quality Check

## 来源

- Product: [Go 代码质量规约治理 / 回归门禁](../../../product/go-code-quality-governance.md#回归门禁)
- Module Key: `regression-gates`
- Story Key: `regression-gates-pr-quality-check`
- Backend: [Review Pipeline 与自动修复](../../../backend/rule-engine-and-autofix.md)
- Quality: [Go 代码质量基线 / 门禁分层](../../../quality/go-code-quality-baseline.md#门禁分层)

## 目标

PR 合入前自动发现违规，并以可读报告阻断不合格变更。

## 范围

包含：

- `ci` profile。
- format check，可由内置 `go.format` 或 `golangci-lint` formatter 承载。
- lint，优先通过 `golangci-lint` adapter 承载。
- arch。
- test，作为独立 `go.test` step 承载。
- 轻量 security。
- 报告 artifact。

不包含：

- 完整 nightly 扫描。
- GitHub App 安装流程。
- 长期趋势分析。

## 依赖

- Backend: pipeline profile、report adapter、统一结果模型。
- Quality: PR 门禁和安全扫描分层策略。

## 任务

- 定义 `ci` profile 示例。
- 支持 report artifact 输出。
- 聚合 gate status。
- 支持 PR 阶段轻量安全检查。
- 输出适合 CI 日志阅读的摘要。

## 验收标准

- `ci` profile 失败时返回非零退出码。
- 违规结果能定位 adapter、step、规则和文件位置。
- 轻量安全检查可配置启停。
- 报告 artifact 可被 CI 上传。

## 测试

- 使用 fixture 验证 CI profile 成功和失败。
- 验证报告 artifact 内容。
- 验证轻量 security step 的失败策略。

## 完成定义

- 满足 PR 检查门禁要求。
