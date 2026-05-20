# Backend Decisions

## B-1 通用规则优先复用成熟 Go 工具链

Status: accepted

决策：格式化、import 整理、静态检查、安全扫描和漏洞扫描优先接入成熟 Go 工具，不重复实现。

理由：成熟工具覆盖面广、社区验证充分、CI 集成稳定。自研应集中在团队特定规约和架构边界。

## B-2 自定义语义规则通过 go/analysis 承载

Status: accepted

决策：需要理解 Go 语法和类型的团队自定义语义规则使用 `golang.org/x/tools/go/analysis` 实现，并通过 diagnostic 和 `SuggestedFix` 表达问题与修复。

理由：自定义规约需要 Go AST 和类型信息才能避免误判。字符串替换无法可靠区分字段读取、写入、取地址和同名成员。

## B-3 平台提供自己的 CLI，并把 golangci-lint 作为 adapter 调用

Status: accepted

决策：平台第一版提供自己的 `go-review` CLI、adapter 运行时和 pipeline 调度器。`golangci-lint` 不作为平台本体，而是由 `go.lint` adapter 调用。

理由：独立 CLI 能统一调度格式化、lint、架构、安全、测试、语义规则和报告输出。`golangci-lint module plugin` 更适合扩展 golangci-lint 本身，但会把平台能力绑定到 golangci-lint 的构建和版本体系。

## B-4 核心引擎采用工具无关 adapter 和 pipeline DAG

Status: accepted

决策：核心引擎采用工具无关 adapter 接口和 pipeline DAG 调度模型。`golangci-lint`、`go test`、`gosec`、`govulncheck`、自研 analyzer 和任意外部命令都只是 adapter。pipeline step 负责声明依赖、并行、超时、失败策略和 artifact。

理由：通用 Go code-review 平台必须能接入任意工具并定义执行顺序。DAG 模型比固定线性流程更适合表达 format -> lint -> test、并行安全扫描、失败跳过下游和自动修复后重跑依赖 step。

## B-5 第一版配置采用简单稳定 YAML schema

Status: accepted

决策：第一版使用 YAML 配置 adapter 和 pipeline。配置先覆盖 adapter ID、命令、参数、工作目录、环境变量、输出解析、step 顺序、依赖、失败策略、是否允许自动修复和 profile。

理由：YAML 对目标用户直观，适合手写和代码审查。第一版 schema 保持简单，可以先保证本地和 CI 复用同一配置，后续再扩展缓存、远程 adapter 和复杂条件。
