# STORY-011 Regression Gates Scheduled Full Regression

## 来源

- Product: [Go 代码质量规约治理 / 回归门禁](../../../product/go-code-quality-governance.md#回归门禁)
- Module Key: `regression-gates`
- Story Key: `regression-gates-scheduled-full-regression`
- Backend: [Review Pipeline 与自动修复](../../../backend/rule-engine-and-autofix.md)
- Quality: [E2E Coverage Baseline](../../../quality/e2e-coverage-baseline.md)

## 目标

团队能定时发现慢速、全量、安全和长期质量问题，并产出可追踪报告。

## 范围

包含：

- `nightly` profile。
- full lint。
- full test。
- race。
- full security。
- 长耗时自定义 rules。
- 定时报告 artifact。

不包含：

- 调度平台实现。
- 告警系统。
- 趋势数据库。

## 依赖

- Backend: pipeline profile、长耗时 step、artifact 输出。
- Quality: nightly 全量安全扫描策略。

## 任务

- 定义 `nightly` profile 示例。
- 支持全量 steps 和长超时配置。
- 输出完整报告 artifact。
- 标记 nightly-only steps。
- 支持失败摘要。

## 验收标准

- `nightly` profile 可以运行全量和长耗时 steps。
- full security 可以与 full test 分离配置。
- 失败报告包含失败 step、原因和 artifact 路径。
- nightly 配置不影响 local 和 ci profile。

## 测试

- 使用 fixture 验证 nightly profile。
- 验证长超时配置和 nightly-only steps。
- 验证失败摘要。

## 完成定义

- 满足定时全量回归质量基线。
