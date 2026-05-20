# STORY-001 Tool Adapter Platform Core

## 来源

- Product: [Go 代码质量规约治理 / 工具接入平台](../../../product/go-code-quality-governance.md#工具接入平台)
- Module Key: `tool-adapter-platform`
- Story Key: `tool-adapter-platform-core`
- Backend: [Review Pipeline 与自动修复](../../../backend/rule-engine-and-autofix.md)
- Quality: [Go 代码质量基线](../../../quality/go-code-quality-baseline.md)

## 目标

用户可以通过一个配置文件接入多个不同工具，并通过统一入口运行这些工具。

## 范围

包含：

- adapter 注册。
- 配置读取。
- 外部命令执行。
- 基础结果归一。
- 通用 `cmd` adapter。

不包含：

- 复杂规则实现。
- 远程 adapter 市场。
- 完整报告平台。

## 依赖

- Backend: adapter 生命周期、通用工具接入契约、统一结果模型。
- Quality: adapter 启停、参数、输出解析和超时 fixture。

## 任务

- 定义 adapter 注册接口和最小 adapter 元数据。
- 定义第一版 YAML 配置结构。
- 实现通用 `cmd` adapter 的命令执行和退出码处理。
- 将 adapter 输出归一成基础 result 结构。
- 提供最小 CLI 入口运行配置中的 adapters。

## 验收标准

- 一个配置文件能声明并运行至少两个 adapters。
- `cmd` adapter 可以运行一个外部命令并捕获退出码、stdout、stderr。
- 成功命令返回通过结果，失败命令返回失败结果。
- adapter 运行结果包含 adapter ID、step ID 和 gate status。

## 测试

- 使用 fixture 配置验证 adapter 注册和执行。
- 使用成功和失败命令验证退出码处理。
- 使用快照或 golden 输出验证结果结构。

## 完成定义

- 符合 [Definition of Done](../../../quality/definition-of-done.md) 中 Adapter 与 Pipeline 配置、检测能力和质量证据要求。
