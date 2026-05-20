# STORY-007 Policy And Autofix Safety Levels

## 来源

- Product: [Go 代码质量规约治理 / 策略与安全自动修复](../../../product/go-code-quality-governance.md#策略与安全自动修复)
- Module Key: `policy-and-autofix`
- Story Key: `policy-and-autofix-safety-levels`
- Backend: [自动修复契约](../../../backend/rule-engine-and-autofix.md#自动修复契约)
- Quality: [Go 代码质量基线 / 自动修复基线](../../../quality/go-code-quality-baseline.md#自动修复基线)

## 目标

用户知道哪些违规会被自动修、哪些只生成建议、哪些只能报告。

## 范围

包含：

- `safe` 策略。
- `review` 策略。
- `report-only` / `none` 策略。
- adapter 和规则级修复策略配置。

不包含：

- 实际复杂代码改写实现。
- UI 审批流。
- 跨文件重构。

## 依赖

- Backend: 自动修复三档契约。
- Quality: 自动修复允许和禁止基线。

## 任务

- 定义修复安全级别枚举。
- 在 result schema 中携带 fix safety。
- 在配置中支持 adapter 和 rule 级默认策略。
- 在 CLI 输出中展示修复策略。
- 阻止 `review` 和 `none` 策略被默认自动应用。

## 验收标准

- 每条可修复结果都标明 `safe`、`review` 或 `none`。
- `safe` 可以在显式 fix 命令下应用。
- `review` 只输出建议，不默认修改。
- `none` 只报告。

## 测试

- 覆盖三种修复策略。
- 验证非 safe 修复不会被自动应用。

## 完成定义

- 满足 [Go 代码质量基线](../../../quality/go-code-quality-baseline.md#自动修复基线)。
