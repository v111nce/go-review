# E2E Coverage Baseline

当前产品能力是 CLI 和 CI 编排平台，没有已确认的 UI 表面，因此本阶段不定义浏览器 E2E 覆盖。质量基线转向 CLI、adapter、pipeline 和报告输出的可执行验证。

## 覆盖矩阵

| Module | Product Source | Frontend Source | Backend Source | Implementation Story | Tier Requirement | Baseline | High Risk | Status |
| --- | --- | --- | --- | --- | --- | --- | --- | --- |
| `tool-adapter-platform` | [产品能力](../product/go-code-quality-governance.md#工具接入平台) | [无 UI 面](../frontend/README.md) | [Review Pipeline 与自动修复](../backend/rule-engine-and-autofix.md) | [STORY-001](../implementation/stories/go-code-quality-governance/STORY-001-tool-adapter-platform-core.md) / [STORY-002](../implementation/stories/go-code-quality-governance/STORY-002-tool-adapter-platform-output-normalization.md) | CLI / adapter 验证 | adapter 注册、命令执行、输出归一 | 工具输出解析不稳定 | draft |
| `review-pipeline` | [产品能力](../product/go-code-quality-governance.md#review-pipeline-编排) | [无 UI 面](../frontend/README.md) | [Review Pipeline 与自动修复](../backend/rule-engine-and-autofix.md) | [STORY-003](../implementation/stories/go-code-quality-governance/STORY-003-review-pipeline-dag-execution.md) / [STORY-004](../implementation/stories/go-code-quality-governance/STORY-004-review-pipeline-profiles.md) | CLI / pipeline 验证 | 顺序、并行、依赖、失败策略 | DAG 调度和重跑逻辑错误 | draft |
| `custom-rule-adapters` | [产品能力](../product/go-code-quality-governance.md#自定义规约-adapter) | [无 UI 面](../frontend/README.md) | [Review Pipeline 与自动修复](../backend/rule-engine-and-autofix.md) | [STORY-005](../implementation/stories/go-code-quality-governance/STORY-005-custom-rule-adapters-sdk.md) / [STORY-006](../implementation/stories/go-code-quality-governance/STORY-006-custom-rule-adapters-command.md) | CLI / adapter fixture 验证 | 自定义 analyzer、通用 `cmd` adapter、统一报告 | 语义规则误伤或外部命令输出不稳定 | draft |
| `policy-and-autofix` | [产品能力](../product/go-code-quality-governance.md#策略与安全自动修复) | [无 UI 面](../frontend/README.md) | [Review Pipeline 与自动修复](../backend/rule-engine-and-autofix.md) | [STORY-007](../implementation/stories/go-code-quality-governance/STORY-007-policy-and-autofix-safety-levels.md) / [STORY-008](../implementation/stories/go-code-quality-governance/STORY-008-policy-and-autofix-transaction.md) | CLI / golden 验证 | safe / review / report-only 三档修复策略 | 自动修复误改业务逻辑 | draft |
| `regression-gates` | [产品能力](../product/go-code-quality-governance.md#回归门禁) | [无 UI 面](../frontend/README.md) | [Review Pipeline 与自动修复](../backend/rule-engine-and-autofix.md) | [STORY-009](../implementation/stories/go-code-quality-governance/STORY-009-regression-gates-local-fast-check.md) / [STORY-010](../implementation/stories/go-code-quality-governance/STORY-010-regression-gates-pr-quality-check.md) / [STORY-011](../implementation/stories/go-code-quality-governance/STORY-011-regression-gates-scheduled-full-regression.md) | CLI / CI 验证 | fast、ci、nightly profile 和报告产物 | 本地、PR、定时任务使用不同配置导致结果漂移 | draft |

## Selector 要求

当前无 UI 面，无 selector 要求。后续如果新增 Web 控制台、PR 报告页或质量趋势页面，需要在 `docs/frontend/` 补充页面文档，并把本基线扩展为三档 E2E 覆盖。

## 测试文件

当前无浏览器 E2E；CLI / adapter / pipeline 验证使用以下 fixture 和 golden artifact 作为第一批可执行基线：

| Test Surface | Files | Stories | Purpose |
| --- | --- | --- | --- |
| Regression gate fixture config | `testdata/fixtures/regression-gates/configs/go-review.yaml` | STORY-003, STORY-004, STORY-009, STORY-010, STORY-011 | Defines fast, ci, main, and nightly profiles against one shared adapter/pipeline contract. |
| Compliant project | `testdata/fixtures/regression-gates/compliant-project/` | STORY-009, STORY-011 | Positive fixture for fast and nightly pass smoke checks. |
| Violating project | `testdata/fixtures/regression-gates/violating-project/` | STORY-010 | Negative fixture for CI gate failure evidence. |
| Fake external tool | `testdata/fixtures/regression-gates/scripts/fake-tool.sh` | STORY-006, STORY-010, STORY-011 | Deterministic `cmd` adapter fixture for pass/fail/timeout behavior. |
| Golden reports | `testdata/fixtures/regression-gates/expected-reports/` | STORY-002, STORY-010, STORY-011 | Stable report snapshots for terminal and JSON artifact checks. |

后续实现代码后，本节应继续补充真实 Go test 文件路径和 CI artifact 路径。
