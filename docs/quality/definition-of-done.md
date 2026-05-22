# Definition of Done

本完成定义用于判断 Go 规约治理能力是否达到可交付状态。

## Adapter 与 Pipeline 配置

- 每个常用 adapter 都有稳定 ID、职责、默认 profile、输出格式和失败策略。
- 通用 `cmd` adapter 可以接入非内置外部工具。
- pipeline 可以表达顺序、并行、依赖、超时和失败策略。
- 每条规则都有稳定 ID、类别、严重级别、作用范围、处理方式和示例。
- 每条规则明确标注 `autofix` 策略：`none`、`safe` 或 `manual-review`。
- 项目配置可以启用、关闭和调整 adapter 严重级别。

## 检测能力

- 通用 Go 规则可通过 `go.lint` adapter 复用 `golangci-lint`，格式/import 修复可由内置 `go.format` 或 `golangci-lint` formatters 执行。
- 架构依赖规则能阻断违规 import，并可由 `depguard`、依赖图检查或自研 `go/analysis` analyzer 承载。
- 自定义语义规则能通过 `go.semantic` 运行 `go/analysis` analyzer，基于 AST 和类型信息定位违规。
- 测试回归通过独立 `go.test` step 执行，不依赖 linter runner。
- 每个违规报告包含 adapter ID、step ID、规则 ID、位置、原因和建议。

## 自动修复能力

- 格式化和 import 顺序可以自动修复。
- 语义安全的局部改写可以自动修复。
- 危险场景只报告，不修改代码。
- 自动修复后必须运行格式化和目标测试。
- 修复失败时有清晰错误报告，不把失败标记为已修复。

## 回归门禁

- 本地快速检查 profile 存在并可复用。
- PR 检查运行 lint、测试、架构和必要安全扫描 steps。
- 主分支合入依赖 required status checks。
- 定时全量回归运行全量测试、race、漏洞扫描或长耗时 adapter。

## 文档架构图

- Product 总入口和核心产品能力文档必须包含 Mermaid 架构图，表达用户、能力模块和质量闭环。
- Backend 总入口和核心后端主题文档必须包含 Mermaid 架构图，表达运行对象、adapter、pipeline、结果、修复和报告之间的关系。
- Mermaid 图必须使用正文已有术语和 module key；图示用于建立理解框架，不能替代契约表、验收信号或测试证据。

## 质量证据

- 至少有一组违规 fixture 和一组合规 fixture。
- 常用 adapter 有启停和失败样例测试。
- pipeline 调度有顺序、并行、依赖和失败策略测试。
- 自定义语义 adapter 有正反例测试。
- 自动修复有 golden file 测试。
- CI 配置和本地命令使用同一套 pipeline 配置。
- 失败样例能证明门禁会阻断违规代码。
