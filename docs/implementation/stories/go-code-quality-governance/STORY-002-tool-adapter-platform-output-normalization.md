# STORY-002 Tool Adapter Platform Output Normalization

## 来源

- Product: [Go 代码质量规约治理 / 工具接入平台](../../../product/go-code-quality-governance.md#工具接入平台)
- Module Key: `tool-adapter-platform`
- Story Key: `tool-adapter-platform-output-normalization`
- Backend: [Review Pipeline 与自动修复](../../../backend/rule-engine-and-autofix.md#统一结果模型)
- Quality: [Go 代码质量基线](../../../quality/go-code-quality-baseline.md#违规报告基线)

## 目标

用户可以看到来自不同工具的统一 review 结果，而不是分别理解每个工具自己的输出格式。

## 范围

包含：

- 文本输出基础解析。
- JSON 输出基础解析。
- `go test` 输出基础解析。
- SARIF 输入的最小字段映射。
- violation、test、artifact 三类统一结果。

不包含：

- 所有第三方工具的完整解析器。
- 复杂 UI 报告。
- 跨运行趋势分析。

## 依赖

- Backend: 统一结果模型、adapter 输出字段。
- Quality: 违规报告基线和高风险漏测项。

## 任务

- 定义 normalized result schema。
- 为 `cmd` adapter 提供退出码和文本输出的默认映射。
- 增加 JSON parser 配置能力。
- 增加 `go test` 基础输出解析。
- 增加 SARIF 到统一结果的最小映射。

## 验收标准

- 文本、JSON、`go test` 和 SARIF 样例都能转成统一结果。
- 每条违规至少包含 adapter ID、step ID、位置、原因和 gate status。
- 无法解析的输出仍作为 artifact 保留。

## 测试

- 使用 fixture 覆盖文本、JSON、`go test`、SARIF 样例。
- 使用格式异常样例验证不会丢失原始输出。

## 完成定义

- 结果结构满足 [Go 代码质量基线](../../../quality/go-code-quality-baseline.md#违规报告基线)。
