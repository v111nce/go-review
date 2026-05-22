# STORY-009 Regression Gates Local Fast Check

## 来源

- Product: [Go 代码质量规约治理 / 回归门禁](../../../product/go-code-quality-governance.md#回归门禁)
- Module Key: `regression-gates`
- Story Key: `regression-gates-local-fast-check`
- Backend: [Review Pipeline 与自动修复](../../../backend/rule-engine-and-autofix.md)
- Quality: [Go 代码质量基线 / 门禁分层](../../../quality/go-code-quality-baseline.md#门禁分层)

## 目标

开发者提交前能快速获得格式、lint 和安全自动修复反馈。

## 范围

包含：

- `fast` profile。
- 快速 format，可由内置 `go.format` 或 `golangci-lint` formatter 承载。
- fast lint，优先通过 `golangci-lint` adapter 承载。
- 受影响测试占位，测试仍由独立 `go.test` step 承载。
- 可安全自动修复。

不包含：

- 完整安全扫描。
- 全量 race 测试。
- PR 评论输出。

## 依赖

- Backend: profile、pipeline、safe fix。
- Quality: 本地快速检查基线。

## 任务

- 定义 `fast` profile 示例。
- 支持 `go-review check --profile fast`。
- 支持 `go-review fix --profile fast`。
- 输出本地可读摘要。
- 失败时返回非零退出码。

## 验收标准

- 本地 profile 能运行快速 steps。
- 可安全修复的问题能通过 fix 命令应用。
- 失败结果包含明确 step 和原因。

## 测试

- 使用 fixture 验证 fast profile。
- 验证 check 和 fix 命令行为。

## 完成定义

- 满足本地快速检查场景的质量基线。
