# Product Decisions

## P-1 先落地工具化规约治理而不是先做 AI agent

Status: accepted

决策：当前阶段忽略 code-review agent，把重点放在可执行规约、检测、自动修复和回归门禁。

理由：Go 质量约束中的格式、lint、架构依赖、测试和安全扫描都有成熟确定性工具。先建立工具基线能更快形成稳定门禁，也能为后续 agent 提供可靠输入。

## P-2 自动修复只覆盖语义安全子集

Status: accepted

决策：自动修复必须限定在能通过 AST、类型信息、读写位置和测试验证证明安全的局部改写；无法证明安全的问题只报告。

理由：企业规约需要长期可信。盲目自动改写架构和业务逻辑会引入隐藏风险，降低团队对工具的信任。

## P-3 所有质量能力都采用 adapter 形态

Status: accepted

决策：格式化、lint、架构检查、安全扫描、测试回归、语义规则和报告输出都按 adapter 接入。核心平台只负责 adapter 发现、配置、调度、结果归一、修复事务和门禁输出。

理由：用户尚未确定哪些规则应该内置。adapter 形态能先定义稳定扩展边界，让常用能力和团队自定义能力都可组合、可启停、可版本化。

## P-4 核心能力升级为工具无关的 review pipeline 编排

Status: accepted

决策：产品核心不是某一组插件，而是工具无关的 Go code-review 编排平台。任意检查、修复、测试、安全、架构和报告工具都可以通过 adapter 接入；项目通过 pipeline 定义执行顺序、并行关系、依赖关系、失败策略和 profile。

理由：用户目标是通用 Go code-review，不能限定死一种工具。编排层能保留现有工具投资，同时让不同项目按自己的质量策略组合工具。

## P-5 第一版交付常用 adapter，不做 adapter 市场

Status: accepted

决策：第一版交付 `cmd`、`go.format`、`go.lint`、`go.arch`、`go.security`、`go.test`、`go.semantic`、`report.github` 这组常用 adapter。`go.lint` / format 类能力优先复用 `golangci-lint`，`go.semantic` 长期使用 `go/analysis` analyzer，`go.test` 保持独立 step。第一版只支持项目内本地 adapter，不提供 adapter 市场或远程安装能力。

理由：这组 adapter 已覆盖通用 Go code-review 的基本闭环。本地 adapter 足以验证接入模型、pipeline 编排和门禁体验；市场和远程安装属于后续分发能力。复用 `golangci-lint` 和 `go/analysis` 可以避免重复造通用 lint/AST 分析轮子，让产品价值集中在统一编排、报告、门禁和安全修复事务。

## P-6 Product 文档必须包含 Mermaid 架构图

Status: accepted

决策：Product 总入口和核心产品能力文档必须包含 Mermaid 架构图，用用户、能力模块、profile、结果和门禁闭环解释产品。

理由：本项目是框架型工具，仅靠表格和段落不容易让读者快速理解“谁配置、平台编排什么、结果如何回到用户”。Product 图负责先建立产品心智，再由模块表和 Story 候选展开细节。
