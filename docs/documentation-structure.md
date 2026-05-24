# Documentation and Repository Structure

本文档说明本项目的目录结构、文档归属和运行代码边界。它是 `docs/README.md` 的补充：`README` 负责导航，本文件负责解释“某类内容应该放在哪里、为什么放在那里”。

## 总体分层

| 路径 | 类型 | 作用 | 维护要求 |
| --- | --- | --- | --- |
| `cmd/go-review/` | CLI 入口 | 解析命令、初始化默认配置、发现项目根目录、触发 engine 运行、输出退出码和用户可见信息。 | 只保留命令层编排；不要把 adapter、规则 catalog 或报告实现写进 CLI。 |
| `internal/` | 核心实现 | 配置解析、pipeline、adapter runtime、fix 事务、结果模型、报告生成和规则 catalog CRUD。 | 新增生产代码需要详细中文注释，说明边界、失败策略和与外部工具的关系。 |
| `rules/` | 规则数据 | `go-rules.json` 是 Go 规范 catalog 的结构化数据源，报告中的 `rule_id` 需要能回到这里。 | 可工具化维护；非 D 类 candidate 规则应标明已实现路径。 |
| `.go-review/` | 本仓库自用配置 | 本项目运行 go-review 时使用的配置、semantic 配置、报告和 artifact。 | 用户配置样例以生成模板和 docs 为准；本目录中的 report/artifact 不作为产品文档。 |
| `docs/` | 权威文档面 | 产品、后端、质量、实现 story、接入、发布和 ADR 的最新可读结论。 | 读者理解当前状态不应依赖 `docs/_process/`。 |
| `docs/_process/` | 过程记录 | 讨论、决策草稿、主题 backlog、文档迁移过程和质量过程配置。 | 非权威；正式结论必须沉淀回 `docs/` 对应目录。 |
| `examples/` | 消费方示例 | 展示其他 Go 项目如何接入 go-review，包括 CI 和示例工程。 | 示例可以故意包含 fixture，但要清楚标注用途。 |
| `integration/` | 集成测试 | 端到端验证 CLI、配置、报告和 fixture 行为。 | 测试应尽量使用隔离临时目录，避免污染仓库工作区。 |
| `testdata/` | 测试数据 | 回归 fixture、故意失败样例和稳定输入。 | 默认运行时会排除，避免被格式化/语义检查误扫。 |
| `AGENTS.md` | LLM review 约束 | C 类无法完全工具化的规范进入 LLM review checklist。 | 不能伪装成确定性工具 gate；必须作为审阅提示和报告依据。 |

## `internal/` 代码目录

| 路径 | 职责 | 典型内容 | 不应承载 |
| --- | --- | --- | --- |
| `internal/adapter/` | adapter 能力枚举和基础类型。 | `check`、`fix`、`test`、`scan`、`report` 等 capability 定义。 | 具体工具执行逻辑。 |
| `internal/config/` | `go-review.yaml` 的轻量 YAML 解析、schema 校验和默认值归一。 | adapter/step/profile/artifact 配置、`args` 列表解析、`on_fail` / `fix_safety` 解析。 | 运行时调度或工具执行。 |
| `internal/engine/` | adapter runtime 和 pipeline 执行核心。 | `cmd` adapter、`go.lint`、`go.semantic`、DAG 执行、artifact 写入、结果归一。 | CLI 参数解析和文档生成。 |
| `internal/engine/testdata/` | engine 测试专用 fixture 和假工具。 | `fake-golangci-lint`。 | 正式默认配置或用户文档。 |
| `internal/fix/` | 自动修复事务边界。 | 快照、回滚、修复前后校验。 | 非安全修复策略。 |
| `internal/pipeline/` | pipeline 计划和依赖顺序。 | step 依赖、跳过策略、失败传播。 | adapter 具体执行。 |
| `internal/report/` | 报告渲染。 | Markdown / LLM report、中文说明、summary。 | 规则检测逻辑。 |
| `internal/result/` | 统一结果模型。 | gate status、fix safety、artifact、violation。 | 工具配置解析。 |
| `internal/rulecatalog/` | 规则 catalog CRUD、校验和 Markdown 渲染。 | `rules/go-rules.json` 的读取、增删改查、validate、render。 | 实际 lint/semantic 检查。 |

## `docs/` 权威文档目录

| 路径 | 归属 | 内容边界 |
| --- | --- | --- |
| `docs/README.md` | 文档入口 | 文档地图、当前结论、主要导航。 |
| `docs/documentation-structure.md` | 文档/仓库结构说明 | 目录职责、内容归属、维护约束。 |
| `docs/product/` | 产品能力源 | 用户价值、能力范围、module key、Story 候选和验收信号。 |
| `docs/frontend/` | UI / 交互 | 当前项目暂无 UI 面，保留 canonical 入口；未来放页面地图、布局、交互状态。 |
| `docs/backend/` | 技术契约 | adapter 生命周期、配置 schema、pipeline、语义规则、自动修复、报告契约。 |
| `docs/implementation/` | 可执行 story | 从 product Story 候选物化出的实现切片、任务、验收和进度。 |
| `docs/quality/` | 质量治理 | Go 规则 catalog、检查基线、DoD、E2E/集成测试策略。 |
| `docs/adoption/` | 消费方接入 | 其他 Go 项目如何安装、初始化配置、接入 CI、阅读报告。 |
| `docs/adr/` | 架构决策 | 长期有效且影响面大的设计取舍。 |
| `docs/glossary.md` | 术语表 | 稳定术语、缩写和项目内专有名词。 |
| `docs/open-questions.md` | 未决问题 | 当前仍需确认的问题，关闭后应迁回正式文档或 ADR。 |
| `docs/release.md` | 发布流程 | tag、二进制、checksum、version stamping。 |
| `docs/quickstart.md` | 快速上手 | 本地安装、初始化和首次运行路径。 |

## 质量规则落地目录关系

| 规则类别 | 权威来源 | 执行位置 | 报告关联 |
| --- | --- | --- | --- |
| A 类：golangci-lint 可覆盖 | `rules/go-rules.json`、`docs/quality/go-rule-catalog.md` | `internal/engine/go_lint.go` 调用 `golangci-lint fmt/run`。 | `golangciLinterRuleID` / formatter 推断映射到 `rule_id`。 |
| B 类：确定性语义规则 | `rules/go-rules.json`、`.go-review/semantic/*.yaml` | `internal/engine/semantic.go` 中的 `go/analysis` analyzer。 | analyzer diagnostic category 映射到 `rule_id`。 |
| C 类：LLM review | `AGENTS.md`、`rules/go-rules.json` | LLM 审阅 checklist，不是确定性 gate。 | catalog 中 `adapter=llm.review`、`tool_rules=[AGENTS.md]`。 |
| D 类：candidate | `rules/go-rules.json` | 暂不实现。 | `implemented=false`，避免误报为已落地能力。 |

## 文档归位规则

- 用户可感知能力、范围和验收信号放 `docs/product/`。
- adapter、pipeline、schema、运行对象、自动修复和报告契约放 `docs/backend/`。
- 检查矩阵、规则 catalog、DoD、测试策略放 `docs/quality/`。
- 可执行任务和 story 进度放 `docs/implementation/`。
- 临时讨论、审计过程、迁移记录放 `docs/_process/`，但不能作为当前事实的唯一来源。
- 新增重大取舍时优先补 ADR；新增术语时同步 `docs/glossary.md`。

## 代码注释约定

- 新增生产代码必须写中文注释，说明“为什么这样做”和“边界是什么”，而不是只复述函数名。
- 对外部工具桥接代码要解释外部工具负责什么、go-review 负责什么。
- 对轻量解析器、AST analyzer、自动修复和失败继续策略，要写明支持的子集和故意不支持的情况。
- 测试代码至少在新增场景测试前说明该测试锁定的行为，避免后续维护者误删 fixture。
