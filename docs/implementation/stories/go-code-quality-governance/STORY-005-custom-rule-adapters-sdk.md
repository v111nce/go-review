# STORY-005 Custom Rule Adapters SDK

## 来源

- Product: [Go 代码质量规约治理 / 自定义规约 Adapter](../../../product/go-code-quality-governance.md#自定义规约-adapter)
- Module Key: `custom-rule-adapters`
- Story Key: `custom-rule-adapters-sdk`
- Backend: [常用 Adapter / go.semantic](../../../backend/rule-engine-and-autofix.md#常用-adapter)
- Quality: [Definition of Done](../../../quality/definition-of-done.md)

## 目标

团队可以编写自己的规则 adapter，并把自定义编码规约接入统一 pipeline。

## 范围

包含：

- Go 语义规则 adapter 接口。
- 示例规则 adapter。
- 本地运行方式。
- 正反例测试样例。

不包含：

- 远程 adapter 市场。
- 动态加载不可信代码。
- 复杂规则 DSL。

## 依赖

- Backend: `go.semantic` adapter、统一结果模型。
- Quality: 自定义语义 adapter 正反例测试。

## 任务

- 定义自定义规则 adapter 的输入输出接口。
- 提供一个最小示例规则。
- 将示例规则输出映射到统一 violation。
- 提供 fixture 测试结构。
- 编写自定义 adapter 使用说明。

## 验收标准

- 示例 adapter 能检测一个违规样例。
- 示例 adapter 对合规样例不报错。
- 结果包含 adapter ID、rule ID、位置、原因和建议。
- 示例 adapter 可以被 pipeline step 调用。

## 测试

- 使用正反例 fixture 验证规则行为。
- 验证 adapter 输出被归一为 violation。

## 完成定义

- 满足 [Definition of Done](../../../quality/definition-of-done.md) 中自定义语义 adapter 质量证据要求。
