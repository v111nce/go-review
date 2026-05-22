# Go 代码质量基线

本基线定义通用 Go code-review 编排平台需要覆盖的检查类型、常用 adapter、自动修复策略和验证方式。它不定义产品范围，也不替代具体项目的业务测试。

## 覆盖矩阵

| 检查类别 | 目标 | 推荐 adapter / 工具 | 自动修复 | 门禁级别 |
| --- | --- | --- | --- | --- |
| 格式化 | 统一代码格式 | `go.format` / `go.lint`：短期 `gofmt`，长期优先复用 `golangci-lint` formatters（`gofmt`、`gofumpt`） | 是 | PR 必须通过 |
| import 整理 | 删除未用 import，统一分组 | `go.format` / `go.lint`：`goimports`、`gci`，长期优先由 `golangci-lint` 聚合 | 是 | PR 必须通过 |
| 基础静态检查 | 无效代码、未使用代码、错误赋值 | `go.lint`：`golangci-lint` 聚合 `go vet`、`staticcheck`、`unused`、`ineffassign` | 部分 | PR 必须通过 |
| 代码风格 | 命名、注释、复杂度、重复代码 | `go.lint`：`golangci-lint` 聚合 `revive`、`cyclop`、`gocognit`、`dupl` | 部分 | 渐进收紧 |
| 错误处理 | 未检查错误、错误包装 | `go.lint`：`golangci-lint` 聚合 `errcheck`、`errorlint` | 部分 | PR 必须通过 |
| 架构依赖 | package import 方向和禁用依赖 | `go.arch`：`depguard`、依赖图检查、自研 `go/analysis` import boundary analyzer | 否 | PR 必须通过 |
| 自定义语义规则 | 团队业务约束 | `go.semantic`：自研 `go/analysis` analyzer runtime / registry | 安全子集 | 按规则启用 |
| 安全扫描 | 常见安全问题 | `go.security`：`gosec`，可由 `golangci-lint` 聚合或独立命令运行 | 否 | PR 轻量 / nightly 全量 |
| 漏洞扫描 | 依赖漏洞 | `go.security`：`govulncheck` 独立命令 | 否 | PR 轻量 / nightly 全量 |
| 测试回归 | 编译、单测和全量回归 | `go.test`：`go test` | 否 | PR 和定时 |
| 数据竞争 | 并发风险 | `go.test`：`go test -race` | 否 | 定时或关键模块 |
| 报告输出 | PR 评论、检查结果、机器可读报告 | `report.github`、JSON、Markdown、SARIF | 不适用 | PR 和定时 |
| 任意工具接入 | 接入项目已有内部工具 | `cmd` adapter | 取决于工具 | 按配置 |

## 门禁分层

| 场景 | 目标 | 推荐命令 |
| --- | --- | --- |
| 本地快速修复 | 提交前自动整理机械问题 | `go-review fix --profile fast` |
| 本地快速检查 | 开发者提交前发现明显违规 | `go-review check --profile fast` |
| PR 检查 | 合入前阻断质量退化 | `go-review check --profile ci` |
| 定时全量 | 发现慢速、全量和并发问题 | `go-review check --profile nightly` |

## 自动修复基线

允许自动修复：

- 格式化和 import 顺序，可由内置 `go.format` 或 `golangci-lint run --fix` 执行。
- adapter 明确声明为 `safe` 的修复。
- `go/analysis` analyzer 通过 `SuggestedFix` 给出、且 `go.semantic` 能证明作用范围安全的局部改写。

禁止默认自动修复：

- package 移动、目录重组和架构重构。
- 涉及业务逻辑分支变化的代码。
- 赋值、取地址、自增、自减、复合赋值等高风险写操作。
- 会产生重叠 text edit 的修复。
- 修复后无法通过格式化或测试的改写。

## 违规报告基线

每个违规至少包含：

| 字段 | 要求 |
| --- | --- |
| Adapter ID | 稳定且可搜索 |
| Step ID | 来源 pipeline step |
| 规则 ID | 在 adapter 内稳定且可搜索 |
| 位置 | 文件、行、列 |
| 原因 | 说明违反了哪条规约 |
| 建议 | 给出修复方向 |
| 自动修复状态 | `available`、`not-safe`、`not-supported` |
| 豁免方式 | 如果允许豁免，说明格式和审批要求 |

## 高风险漏测项

- 自定义语义规则绕开 `go/analysis`，只用字符串匹配，误伤同名字段或方法。
- 把 `golangci-lint` 当成完整 review pipeline，导致 `go test`、安全扫描、报告和失败策略缺失。
- 自动修复没有区分安全场景和危险场景。
- CI 和本地使用不同版本的 adapter 或底层工具。
- pipeline 只支持线性顺序，无法表达并行、依赖和失败策略。
- `nolint` 没有说明原因，导致长期逃逸。
- 定时全量回归失败后没有所有者和处理流程。
- 架构规则只检查文件路径，不检查真实 import 关系。

## 参考来源

- Go `go/analysis`：https://pkg.go.dev/golang.org/x/tools/go/analysis
- golangci-lint：https://golangci-lint.run/
- golangci-lint linters：https://golangci-lint.run/docs/linters/
- Go race detector：https://go.dev/doc/articles/race_detector
- govulncheck：https://go.dev/doc/tutorial/govulncheck
