# STORY-003 Review Pipeline DAG Execution

## 来源

- Product: [Go 代码质量规约治理 / Review Pipeline 编排](../../../product/go-code-quality-governance.md#review-pipeline-编排)
- Module Key: `review-pipeline`
- Story Key: `review-pipeline-dag-execution`
- Backend: [Review Pipeline 模型](../../../backend/rule-engine-and-autofix.md#review-pipeline-模型)
- Quality: [Definition of Done](../../../quality/definition-of-done.md)

## 目标

用户可以定义工具的先后顺序、并行关系、依赖关系和失败策略。

## 范围

包含：

- pipeline step。
- `depends_on` 依赖。
- 可并行执行的 ready steps。
- `on_fail` 失败策略。
- DAG 校验。

不包含：

- 远程分布式执行。
- 高级缓存。
- 复杂条件表达式语言。

## 依赖

- Backend: Pipeline DAG、step 字段、失败策略。
- Quality: pipeline 调度测试要求。

## 任务

- 定义 pipeline step 配置结构。
- 实现 DAG 构建和环检测。
- 实现 ready step 调度。
- 实现 `stop`、`continue`、`skip_dependents` 失败策略。
- 聚合 pipeline 级 gate status。

## 验收标准

- 配置能表达 `format -> lint -> test`。
- 不互相依赖的 steps 可以并行运行。
- 环形依赖会被拒绝并给出清晰错误。
- step 失败时按 `on_fail` 控制下游行为。

## 测试

- 覆盖线性、并行、菱形依赖和环形依赖。
- 覆盖三种失败策略。
- 验证 pipeline 最终 gate status。

## 完成定义

- 满足 [Definition of Done](../../../quality/definition-of-done.md) 中 pipeline 顺序、并行、依赖和失败策略要求。
