# Glossary

| Term | Definition |
| --- | --- |
| Adapter | 平台接入工具的统一包装，可以运行外部命令、Go analyzer、测试或报告输出 |
| 常用 Adapter | 平台默认提供的常见 Go 工具接入能力 |
| 自定义 Adapter | 团队按项目规则编写或配置的工具接入能力 |
| Review Pipeline | 一组按顺序、并行和依赖关系执行的 code-review steps |
| Step | pipeline 中的一个执行节点，绑定一个 adapter 和参数 |
| Profile | 本地、PR、主分支、nightly 等不同场景的 pipeline 配置 |
| 规约目录 | 团队认可并可执行的一组 Go 代码质量规则 |
| 违规 | 不符合规约的代码、依赖、目录或配置问题 |
| 自动修复 | 工具在语义安全条件满足时应用的代码改写 |
| 只报告违规 | 工具发现问题但不自动修改代码 |
| 回归门禁 | 在本地、PR、主分支或定时任务中运行的质量检查 |
| `module key` | 产品功能模块的稳定英文标识 |
| `go/analysis` | Go 官方工具链中用于构建静态分析器的包；本平台把它作为自定义 Go semantic analyzer 底座 |
| `SuggestedFix` | `go/analysis` 诊断中携带的建议修复，只有证明安全时才可进入自动修复事务 |
| `golangci-lint` | Go 常用聚合型 linter / formatter runner；本平台把它作为 `go.lint` / format 类 adapter 的优先底座，而不是平台本体 |
| `depguard` | 用于限制 Go import 依赖的 linter |
| 外部命令 Adapter | 平台通过启动外部可执行程序接入的 adapter 形态，适合隔离运行和语言无关扩展 |
| Go 语义规则 Adapter | 基于 Go AST 和类型信息实现的规则 adapter，长期由 `go/analysis` analyzer runtime / registry 承载，适合检测普通文本匹配无法可靠判断的 Go 代码语义 |
| `golangci-lint module plugin` | golangci-lint 自身的扩展方式，适合把自定义 linter 深度集成到 golangci-lint，但不作为本平台第一版主扩展面；本平台优先通过 adapter 调用 golangci-lint |
