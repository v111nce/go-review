# STORY-008 Policy And Autofix Transaction

## 来源

- Product: [Go 代码质量规约治理 / 策略与安全自动修复](../../../product/go-code-quality-governance.md#策略与安全自动修复)
- Module Key: `policy-and-autofix`
- Story Key: `policy-and-autofix-transaction`
- Backend: [自动修复契约](../../../backend/rule-engine-and-autofix.md#自动修复契约)
- Quality: [Definition of Done](../../../quality/definition-of-done.md)

## 目标

用户可以安全应用自动修复；修复失败不会留下半完成状态。

## 范围

包含：

- 修复预览。
- text edit 冲突检测。
- 事务式应用。
- 格式化后验证。
- 依赖 step 重跑。
- 失败回滚。

不包含：

- 大规模跨仓库迁移。
- 手工审批 UI。
- 复杂 merge 冲突解决。

## 依赖

- Backend: Fix Transaction Manager、pipeline 依赖重跑。
- Quality: golden file 和失败回滚测试。

## 任务

- 定义 fix transaction 数据结构。
- 实现 text edit 冲突检测。
- 实现应用前预览输出。
- 应用 safe fixes 后运行 formatter。
- 重新运行受影响 steps。
- 任一验证失败时回滚。

## 验收标准

- 无冲突 safe fixes 可以应用。
- 重叠 text edits 被拒绝。
- 格式化失败时回滚。
- 依赖 step 失败时回滚或标记失败，不声称修复完成。

## 测试

- golden file 验证修复前后。
- 覆盖重叠 edit、格式化失败、依赖 step 失败。

## 完成定义

- 自动修复行为满足 [Definition of Done](../../../quality/definition-of-done.md#自动修复能力)。
