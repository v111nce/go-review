# Go Code Review Quality Governance Docs

本目录是 Go 项目 code-review 质量治理方案的权威文档面。当前目标是建立通用 Go code-review 编排平台：不绑定任何单一工具，允许接入任意检查、修复、测试、安全、架构和报告工具，并由项目配置定义执行顺序、依赖关系、失败策略和输出方式。

## 文档地图

| 领域 | 入口 | 内容 |
| --- | --- | --- |
| Product | [product/README.md](product/README.md) | 产品能力、用户价值、功能模块和 Story 候选 |
| Frontend | [frontend/README.md](frontend/README.md) | 当前无 UI 面，保留 canonical 入口 |
| Backend | [backend/README.md](backend/README.md) | 规则引擎、检测器、自动修复和运行契约 |
| Quality | [quality/README.md](quality/README.md) | 质量基线、检查矩阵、完成定义和验证策略 |
| Implementation | [implementation/README.md](implementation/README.md) | 后续从产品 Story 候选物化的执行 story |
| Adoption | [adoption/consumer-project.md](adoption/consumer-project.md) | 其他 Go 项目如何接入 `go-review` 的配置、命令和 CI 模板 |
| Release | [release.md](release.md) | Tag-based release workflow, binary artifacts, checksums, and version stamping |
| ADR | [adr/README.md](adr/README.md) | 长期架构决策 |

## 当前结论

- 核心系统只负责工具接入、配置读取、执行编排、结果归一、修复事务、报告生成和门禁输出。
- 工具接入是开放的：`golangci-lint`、`go test`、`gosec`、`govulncheck`、自研 analyzer 或任意外部命令都只是 adapter。
- 用户可以通过 pipeline 配置定义工具的先后顺序、并行关系、依赖关系、失败后是否继续和不同场景的 profile。
- 第一版提供常用 Go 工具 adapter，但它们不是平台边界。
- 自动修复只覆盖语义安全的子集；不能证明安全的违规只报告，不自动改。

## 问题记录

问题和已关闭决策记录集中维护在 [open-questions.md](open-questions.md)。
