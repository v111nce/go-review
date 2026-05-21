# STORY-004 Review Pipeline Profiles

## 来源

- Product: [Go 代码质量规约治理 / Review Pipeline 编排](../../../product/go-code-quality-governance.md#review-pipeline-编排)
- Module Key: `review-pipeline`
- Story Key: `review-pipeline-profiles`
- Backend: [Review Pipeline 与自动修复](../../../backend/rule-engine-and-autofix.md)
- Quality: [Go 代码质量基线 / 门禁分层](../../../quality/go-code-quality-baseline.md#门禁分层)

## 目标

用户可以按本地、PR、主分支和 nightly 场景使用不同检查强度。

## 范围

包含：

- `fast` profile。
- `ci` profile。
- `main` profile。
- `nightly` profile。
- profile 选择和默认值。

不包含：

- 用户权限系统。
- UI 配置界面。
- 远程配置分发。

## 依赖

- Backend: pipeline 配置和 step 执行。
- Quality: 门禁分层策略。

## 任务

- 在 YAML schema 中定义 profiles。
- 支持 CLI 指定 `--profile`。
- 支持 profile 复用 adapters 和 steps。
- 为缺失 profile 提供清晰错误。
- 提供最小示例配置。

## 验收标准

- `go-review check --profile fast` 只运行本地快速 steps。
- `go-review check --profile ci` 运行 PR 门禁 steps。
- `go-review check --profile nightly` 可以包含全量和长耗时 steps。
- 未定义 profile 时返回可理解错误。

## 测试

- 使用 fixture 配置验证不同 profile 运行不同 steps。
- 验证默认 profile 和显式 profile。

## 完成定义

- 本地、PR 和 nightly profile 能复用同一套 pipeline 配置。
