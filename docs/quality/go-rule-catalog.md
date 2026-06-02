# Go 规则 Catalog

> 本文档由 JSON catalog 生成；请通过 `go-review rules` CRUD 命令维护源数据。

本 catalog 用来把 Go 官方、Google Go Style Guide、Uber Go Style Guide 中可复用的规范沉淀成 `go-review` 可治理的规则目录。它不是说所有规则都已经实现；它先记录来源、规则说明、处理方式、推荐承接和默认启用策略，后续再按优先级接入 `golangci-lint`、`go test`、`govulncheck`、`gosec` 或 `go.semantic`。

## 处理方式定义

catalog 不使用“工具 + 人工混合覆盖”这类模糊状态。原则是：只有确定能由工具稳定处理的规则，才标为工具处理；否则退一步，进入更宽松的 LLM/人工 review 或 candidate。

| 处理方式 | 含义 | go-review 处理方式 |
| --- | --- | --- |
| `tool-golangci` | 已确认可由 golangci-lint 内置 linter/formatter 稳定处理 | 通过 `go.lint` adapter 启用，不自研。 |
| `tool-golangci-config` | 可由 golangci-lint 处理，但必须配置阈值、名单、例外或具体 linter | 先沉淀推荐配置，再按 profile 启用。 |
| `tool-go-test` | 通过 `go test`、example test 或测试运行本身验证 | 保持独立 `go.test` step。 |
| `tool-external` | 更适合独立工具，例如 `govulncheck`、`gosec` | 独立 adapter 或可选 profile。 |
| `tool-semantic` | 已确认需要 AST/type info，且能写成稳定 `go.semantic` analyzer | 进入 `.go-review/rules/semantic-custom.yaml` 支持范围或新增 analyzer。 |
| `llm-review` | 需要上下文判断、原则解释或设计取舍；不适合确定性工具 gate | 写入规范文档，并进入 LLM report / 人工 review checklist；默认不阻断。 |
| `candidate` | 候选规则，仍需验证工具覆盖、误判率和启用成本 | 不默认启用。 |

## 默认启用策略

| 层级 | 默认策略 | 例子 |
| --- | --- | --- |
| `default` | 低争议、确定性强、误判低 | gofmt、goimports、govet、staticcheck、unused、ineffassign、errcheck、go test。 |
| `ci` | PR 可承受、结果稳定 | errorlint、bodyclose、contextcheck、基础 semantic 规则。 |
| `strict` | 团队明确接受后启用 | 函数长度、复杂度、import 边界、变量名长度、接口方法数量。 |
| `nightly` | 慢速或全量扫描 | govulncheck、gosec 全量、race、重型 semantic。 |
| `doc` | 只解释，不阻断 | 接口是否过早抽象、包边界是否清晰、代码是否足够简单。 |

## JSON catalog 与 CRUD

`rules/go-rules.json` 是机器可读源数据，`docs/quality/go-rule-catalog.md` 是面向人阅读的视图。检测报告必须全链路携带稳定 `rule_id`。`implemented` 必须显式标识该规则是否已经被代码实现并有测试覆盖；不能只因为进入 catalog 就算已实现。

CRUD 命令：

```bash
go-review rules list --catalog rules/go-rules.json
go-review rules get team.semantic.max-params --catalog rules/go-rules.json
go-review rules add --catalog rules/go-rules.json --file new-rule.json
go-review rules upsert --catalog rules/go-rules.json --file changed-rule.json
go-review rules delete old.rule.id --catalog rules/go-rules.json
go-review rules validate --catalog rules/go-rules.json
go-review rules render-doc --catalog rules/go-rules.json --out docs/quality/go-rule-catalog.md
```

## 按落地路径分组

本 catalog 的主维护视图按 go-review 的实际落地路径分组，而不是只按规范来源展开。这样能直接看出一条规则应该交给现成工具、自定义 analyzer，还是 LLM/人工 review。

| 分组 | 数量 | 处理原则 |
| --- | ---: | --- |
| lint / 现成工具框架 | 85 | 优先复用 `golangci-lint`、`go test`、`gosec`、`govulncheck` 等；能稳定映射 `rule_id` 后再标 `implemented: true`。 |
| 自定义开发 / go.semantic | 8 | 现成工具覆盖不了，但 AST、import、type info 或确定性控制流能稳定判断时，开发 `go/analysis` analyzer。 |
| LLM / 人工 review | 122 | 需要设计意图、API 语义、并发生命周期、测试诊断质量或项目上下文；进入报告和 checklist，默认不硬阻断。 |
| Candidate | 8 | 待验证底层工具能力、误判率、配置成本和是否值得阻断；确认后迁入前三类。 |

### A. 走 lint / 现成工具框架

这些规则应优先通过 `go.lint`、`go.test` 或独立外部 adapter 承接。`tool-golangci-config` 表示需要先固化 linter、阈值、例外或团队配置。

| Rule ID | 规则说明 | 处理方式 | 推荐承接 | 默认 | 已实现 | 备注 |
| --- | --- | --- | --- | --- | --- | --- |
| `go.official.comment-sentences` | 注释应像完整句子，尤其 exported API 注释要可读。 | `tool-golangci-config` | go.lint / godoclint, godot, revive | strict | yes | 主要针对 doc comment 句式。已通过 go.lint/golangci-lint 编排承接；报告按 linter 或 adapter 输出稳定 rule_id。 已从通用默认 lint 中移出，保留为 strict/团队可选规则。 |
| `go.official.crypto-rand` | 安全随机场景必须使用 crypto/rand，不用 math/rand。 | `tool-golangci` | go.lint / gosec | ci | yes | gosec 输出已由 go.lint 映射为 go.official.crypto-rand，并有 adapter 测试覆盖。已通过 go.lint/golangci-lint 编排承接；报告按 linter 或 adapter 输出稳定 rule_id。 |
| `go.official.doc-comments` | exported API 和 package 至少应具备符合 Go 文档约定的注释。 | `tool-golangci-config` | go.lint / godoclint, revive | ci | yes | 工具只检查存在性和机械格式；注释是否真正有用另走 review。已通过 go.lint/golangci-lint 编排承接；报告按 linter 或 adapter 输出稳定 rule_id。 |
| `go.official.error-strings` | error 文本不要无故大写开头或以标点结尾，便于组合。 | `tool-golangci-config` | go.lint / revive error-strings | ci | yes | 明确启用 revive 对应规则后处理。已通过 go.lint/golangci-lint 编排承接；报告按 linter 或 adapter 输出稳定 rule_id。 |
| `go.official.examples` | example test 一旦存在，必须可编译、可运行并参与 go test。 | `tool-go-test` | go.test / go test | strict | yes | “是否必须写示例”属于文档策略，不由该工具状态承诺。已通过 go.test step 承接，验证已存在测试/example 能参与 go test。 |
| `go.official.gofmt` | 代码必须使用 gofmt 统一格式化，避免人工风格争论。 | `tool-golangci` | go.lint / gofmt | default | yes | 机械格式化，可 safe fix；报告 rule_id 输出 go.official.gofmt。已通过 go.lint/golangci-lint 编排承接；报告按 linter 或 adapter 输出稳定 rule_id。 |
| `go.official.handle-errors` | error 不应被静默丢弃；应处理、返回或明确忽略。 | `tool-golangci` | go.lint / errcheck, govet | default | yes | errcheck 输出会映射为 go.official.handle-errors。已通过 go.lint/golangci-lint 编排承接；报告按 linter 或 adapter 输出稳定 rule_id。 |
| `go.official.identifier-style` | Go 标识符使用 Go 风格大小写，不用 snake_case 或 ALL_CAPS。 | `tool-golangci` | go.lint / revive | strict | yes | revive 输出已由 go.lint 映射为 go.official.identifier-style，并有 adapter 测试覆盖。已通过 go.lint/golangci-lint 编排承接；报告按 linter 或 adapter 输出稳定 rule_id。 已从通用默认 lint 中移出，保留为 strict/团队可选规则。 |
| `go.official.import-dot` | 避免 dot import，除非测试等少数合理场景。 | `tool-golangci` | go.lint / depguard, revive | strict | yes | 少数测试场景可豁免。已通过 go.lint/golangci-lint 编排承接；报告按 linter 或 adapter 输出稳定 rule_id。 已从通用默认 lint 中移出，保留为 strict/团队可选规则。 |
| `go.official.imports` | import 应自动整理、删除未用项并按 Go 习惯分组。 | `tool-golangci` | go.lint / gci, goimports | default | yes | 配置 goimports/gci formatter 时，报告 rule_id 输出 go.official.imports。已通过 go.lint/golangci-lint 编排承接；报告按 linter 或 adapter 输出稳定 rule_id。 |
| `go.official.indent-error-flow` | 先处理错误并提前返回，让正常路径减少缩进。 | `tool-golangci-config` | go.lint / revive | ci | yes | 可检测 else-after-return 等模式。已通过 go.lint/golangci-lint 编排承接；报告按 linter 或 adapter 输出稳定 rule_id。 |
| `go.official.initialisms` | URL、ID、HTTP 等缩写大小写要一致。 | `tool-golangci-config` | go.lint / revive naming | ci | yes | 维护 initialism 列表。已通过 go.lint/golangci-lint 编排承接；报告按 linter 或 adapter 输出稳定 rule_id。 |
| `go.official.naked-returns` | 中等或较长函数避免裸 return，显式返回更清楚。 | `tool-golangci-config` | go.lint / nakedret | strict | yes | 阈值配置。已通过 go.lint/golangci-lint 编排承接；报告按 linter 或 adapter 输出稳定 rule_id。 已从通用默认 lint 中移出，保留为 strict/团队可选规则。 |
| `go.official.named-results` | 不要为了省局部变量而滥用命名返回值；命名应服务文档或 defer 语义。 | `tool-golangci-config` | go.lint / nonamedreturns, revive | strict | yes | 文档化返回值和 defer 修改场景例外。已通过 go.lint/golangci-lint 编排承接；报告按 linter 或 adapter 输出稳定 rule_id。 已从通用默认 lint 中移出，保留为 strict/团队可选规则。 |
| `go.official.no-panic` | 正常错误流程不要 panic，应返回 error 或显式处理。 | `tool-golangci-config` | go.lint / forbidigo, go.semantic, revive | strict | yes | main/init/test helper 例外需配置。已通过 go.lint/golangci-lint 编排承接；报告按 linter 或 adapter 输出稳定 rule_id。 |
| `go.official.package-comments` | package 应有说明整体用途的 package comment。 | `tool-golangci-config` | go.lint / godoclint, revive | ci | yes | package 文档。已通过 go.lint/golangci-lint 编排承接；报告按 linter 或 adapter 输出稳定 rule_id。 |
| `go.official.package-names` | package 名应短小写、有意义，避免 util/common/helper 这类空泛名称。 | `tool-golangci-config` | go.lint / denylist, revive | strict | yes | util/common/helper 等可列黑名单。已通过 go.lint/golangci-lint 编排承接；报告按 linter 或 adapter 输出稳定 rule_id。 |
| `go.test.mark-helpers` | 测试 helper 应调用 t.Helper()，让失败位置指向调用处。 | `tool-golangci` | go.lint / thelper | strict | yes | 稳定检测 t.Helper()。已通过 go.lint/golangci-lint 编排承接；报告按 linter 或 adapter 输出稳定 rule_id。 已从通用默认 lint 中移出，保留为 strict/团队可选规则。 |
| `go.test.no-assert-libraries` | 避免断言库隐藏控制流或产生不足的失败信息。 | `tool-golangci-config` | go.lint / forbidigo, testifylint | strict | yes | 若团队允许 testify 则不启用。已通过 go.lint/golangci-lint 编排承接；报告按 linter 或 adapter 输出稳定 rule_id。 |
| `google.bp.composite-literals` | 复合字面量应清晰，必要时使用字段名。 | `tool-golangci` | go.lint / gofmt, gofumpt, govet | ci | yes | field names 等。已通过 go.lint/golangci-lint 编排承接；报告按 linter 或 adapter 输出稳定 rule_id。 |
| `google.bp.error-w-placement` | 包装错误时 %w 的位置和数量应符合 errors.Is/As 语义。 | `tool-golangci` | go.lint / errorlint | ci | yes | %w 语义可检测。已通过 go.lint/golangci-lint 编排承接；报告按 linter 或 adapter 输出稳定 rule_id。 |
| `google.bp.field-names-test-literals` | 测试中的 struct literal 使用字段名提升可读性和抗变更能力。 | `tool-golangci` | go.lint / govet composites | ci | yes | 外部包 struct 稳定检测。已通过 go.lint/golangci-lint 编排承接；报告按 linter 或 adapter 输出稳定 rule_id。 |
| `google.bp.godoc-formatting` | Godoc 格式应符合 Go 文档渲染习惯。 | `tool-golangci-config` | go.lint / godoclint, godot | ci | yes | 机械格式。已通过 go.lint/golangci-lint 编排承接；报告按 linter 或 adapter 输出稳定 rule_id。 |
| `google.bp.import-ordering` | import 排序和分组交给工具。 | `tool-golangci` | go.lint / gci, goimports | default | yes | 与官方 imports 合并。已通过 go.lint/golangci-lint 编排承接；报告按 linter 或 adapter 输出稳定 rule_id。 |
| `google.bp.naming-conventions` | 命名遵循 Go 习惯而非其他语言风格。 | `tool-golangci-config` | go.lint / revive | strict | yes | 可覆盖机械命名子集。已通过 go.lint/golangci-lint 编排承接；报告按 linter 或 adapter 输出稳定 rule_id。 |
| `google.bp.string-builder` | 多步构造字符串时用 strings.Builder。 | `tool-golangci-config` | go.lint / gocritic, perfsprint | strict | yes | 性能 profile。已通过 go.lint/golangci-lint 编排承接；报告按 linter 或 adapter 输出稳定 rule_id。 |
| `google.bp.util-packages` | 避免 util/common 这类无领域含义的包。 | `tool-golangci-config` | go.lint / package denylist, semantic | strict | yes | util/common 命名治理。已通过 go.lint/golangci-lint 编排承接；报告按 linter 或 adapter 输出稳定 rule_id。 |
| `google.bp.var-initialization` | 变量初始化应简洁，优先利用零值。 | `tool-golangci-config` | go.lint / gofumpt, staticcheck | ci | yes | 可覆盖零值初始化子集。已通过 go.lint/golangci-lint 编排承接；报告按 linter 或 adapter 输出稳定 rule_id。 |
| `google.bp.zero-values` | 声明零值变量时避免冗余初始化。 | `tool-golangci` | go.lint / gofumpt, staticcheck | ci | yes | 冗余初始化。已通过 go.lint/golangci-lint 编排承接；报告按 linter 或 adapter 输出稳定 rule_id。 |
| `google.commentary.comment-sentences` | 注释应是可读句子，首词和标点符合 Go 文档习惯。 | `tool-golangci-config` | go.lint / godot, revive | ci | yes | 句式规则。已通过 go.lint/golangci-lint 编排承接；报告按 linter 或 adapter 输出稳定 rule_id。 |
| `google.commentary.examples` | 示例应可运行，并通过 go test 参与验证。 | `tool-go-test` | go.test / go test | strict | yes | 仅验证已存在示例能运行；是否需要示例不阻断。已通过 go.test step 承接，验证已存在测试/example 能参与 go test。 |
| `google.commentary.named-results` | 命名返回值只在能改善文档或 defer 语义时使用。 | `tool-golangci-config` | go.lint / nonamedreturns | strict | yes | 与官方合并。已通过 go.lint/golangci-lint 编排承接；报告按 linter 或 adapter 输出稳定 rule_id。 |
| `google.errors.error-strings` | 错误文本应便于组合，不大写开头、不带多余标点。 | `tool-golangci-config` | go.lint / revive | ci | yes | 与官方 error strings 合并。已通过 go.lint/golangci-lint 编排承接；报告按 linter 或 adapter 输出稳定 rule_id。 |
| `google.errors.handle-errors` | 错误应被处理、返回或明确忽略。 | `tool-golangci` | go.lint / errcheck | default | yes | 与官方合并。已通过 go.lint/golangci-lint 编排承接；报告按 linter 或 adapter 输出稳定 rule_id。 |
| `google.errors.indent-error-flow` | 错误分支提前返回，减少正常路径缩进。 | `tool-golangci-config` | go.lint / revive | ci | yes | 与官方合并。已通过 go.lint/golangci-lint 编排承接；报告按 linter 或 adapter 输出稳定 rule_id。 |
| `google.imports.dot` | 避免 dot import，防止命名来源不清。 | `tool-golangci` | go.lint / revive | strict | yes | 与官方 dot import 合并。已通过 go.lint/golangci-lint 编排承接；报告按 linter 或 adapter 输出稳定 rule_id。 已从通用默认 lint 中移出，保留为 strict/团队可选规则。 |
| `google.imports.grouping` | import 应按 Go 习惯分组。 | `tool-golangci` | go.lint / gci, goimports | default | yes | 可 safe fix。已通过 go.lint/golangci-lint 编排承接；报告按 linter 或 adapter 输出稳定 rule_id。 |
| `google.imports.renaming` | import alias 只在必要时使用，并保持一致。 | `tool-golangci-config` | go.lint / importas, revive | strict | yes | 必要 alias 才允许。已通过 go.lint/golangci-lint 编排承接；报告按 linter 或 adapter 输出稳定 rule_id。 |
| `google.language.conditionals-loops` | 条件和循环应简洁，避免不必要嵌套。 | `tool-golangci-config` | go.lint / gocritic, revive | ci | yes | 简化控制流。已通过 go.lint/golangci-lint 编排承接；报告按 linter 或 adapter 输出稳定 rule_id。 |
| `google.language.function-formatting` | 函数签名和函数体格式交给 gofmt。 | `tool-golangci` | go.lint / gofmt | default | yes | 机械格式。已通过 go.lint/golangci-lint 编排承接；报告按 linter 或 adapter 输出稳定 rule_id。 |
| `google.language.indentation-confusion` | 缩进不能制造控制流错觉，交给 gofmt 和静态检查处理。 | `tool-golangci` | go.lint / gofmt, staticcheck | default | yes | 主要由 gofmt。已通过 go.lint/golangci-lint 编排承接；报告按 linter 或 adapter 输出稳定 rule_id。 |
| `google.language.literal-braces` | 复合字面量和括号布局交给 gofmt 保持一致。 | `tool-golangci` | go.lint / gofmt | default | yes | 主要由 gofmt 处理。已通过 go.lint/golangci-lint 编排承接；报告按 linter 或 adapter 输出稳定 rule_id。 |
| `google.language.literal-field-names` | 结构体字面量必要时写字段名，避免字段顺序变更导致错误。 | `tool-golangci` | go.lint / govet composites | ci | yes | 只承接 govet composites 能稳定覆盖的外部包 struct literal。已通过 go.lint/golangci-lint 编排承接；报告按 linter 或 adapter 输出稳定 rule_id。 |
| `google.language.no-panic` | 正常错误流程不要 panic。 | `tool-golangci-config` | go.lint / forbidigo, revive | strict | yes | 与官方合并。已通过 go.lint/golangci-lint 编排承接；报告按 linter 或 adapter 输出稳定 rule_id。 |
| `google.language.repeated-type-names` | 复合字面量中避免重复类型名，提升可读性。 | `tool-golangci` | go.lint / gofmt, gofumpt | default | yes | 复合字面量格式。已通过 go.lint/golangci-lint 编排承接；报告按 linter 或 adapter 输出稳定 rule_id。 |
| `google.language.switch-break` | switch 中避免无意义 break 等冗余控制流。 | `tool-golangci-config` | go.lint / gocritic, revive | ci | yes | 冗余 break 可检测。已通过 go.lint/golangci-lint 编排承接；报告按 linter 或 adapter 输出稳定 rule_id。 |
| `google.libs.crypto-rand` | 安全随机场景应使用 crypto/rand。 | `tool-golangci` | go.lint / gosec | ci | yes | 与官方合并。已通过 go.lint/golangci-lint 编排承接；报告按 linter 或 adapter 输出稳定 rule_id。 |
| `google.naming.constant-names` | 常量名使用 MixedCaps，不用 ALL_CAPS 或 kFoo。 | `tool-golangci` | go.lint / revive | strict | yes | MixedCaps，不用 ALL_CAPS/kFoo。已通过 go.lint/golangci-lint 编排承接；报告按 linter 或 adapter 输出稳定 rule_id。 已从通用默认 lint 中移出，保留为 strict/团队可选规则。 |
| `google.naming.getters` | getter 通常不加 Get 前缀。 | `tool-golangci-config` | go.lint / revive | strict | yes | 不默认 Get 前缀。已通过 go.lint/golangci-lint 编排承接；报告按 linter 或 adapter 输出稳定 rule_id。 |
| `google.naming.initialisms` | 常见缩写大小写保持一致。 | `tool-golangci-config` | go.lint / revive | ci | yes | 与官方合并。已通过 go.lint/golangci-lint 编排承接；报告按 linter 或 adapter 输出稳定 rule_id。 |
| `google.naming.package-names` | package 名应短小写、有领域含义。 | `tool-golangci-config` | go.lint / denylist, revive | strict | yes | 与官方 package names 合并。已通过 go.lint/golangci-lint 编排承接；报告按 linter 或 adapter 输出稳定 rule_id。 |
| `google.naming.single-letter-vars` | 单字母变量只适合很短且含义明显的作用域。 | `tool-golangci-config` | go.lint / varnamelen | strict | yes | 短作用域可允许。已通过 go.lint/golangci-lint 编排承接；报告按 linter 或 adapter 输出稳定 rule_id。 已从通用默认 lint 中移出，保留为 strict/团队可选规则。 |
| `google.naming.underscores` | 标识符通常不用下划线，测试函数和少数底层场景例外。 | `tool-golangci-config` | go.lint / revive | strict | yes | 测试函数/生成代码例外。已通过 go.lint/golangci-lint 编排承接；报告按 linter 或 adapter 输出稳定 rule_id。 |
| `google.test.assertions` | 断言库不应隐藏失败上下文或阻断后续检查。 | `tool-golangci-config` | go.lint / forbidigo, testifylint | strict | yes | 与 Go Test Comments 合并。已通过 go.lint/golangci-lint 编排承接；报告按 linter 或 adapter 输出稳定 rule_id。 |
| `google.test.got-before-want` | 测试输出按 got 在前、want 在后。 | `tool-golangci-config` | go.lint / semantic, testifylint | strict | yes | 与 Go Test Comments 合并。已通过 go.lint/golangci-lint 编排承接；报告按 linter 或 adapter 输出稳定 rule_id。 |
| `google.test.helpers` | 测试 helper 应标记 t.Helper 并给出好错误。 | `tool-golangci` | go.lint / thelper | strict | yes | t.Helper 稳定检测。已通过 go.lint/golangci-lint 编排承接；报告按 linter 或 adapter 输出稳定 rule_id。 已从通用默认 lint 中移出，保留为 strict/团队可选规则。 |
| `google.test.use-package-testing` | Go 测试应使用标准 testing 包生态。 | `tool-go-test` | go.test / go test | default | yes | 基础测试框架。已通过 go.test step 承接，验证已存在测试/example 能参与 go test。 |
| `uber.guideline.atomic` | 并发原子值可用封装类型提升类型安全。 | `tool-golangci-config` | go.lint / atomic linters, dep policy | strict | yes | 是否采用 uber atomic 由团队决定。已通过 go.lint/golangci-lint 编排承接；报告按 linter 或 adapter 输出稳定 rule_id。 |
| `uber.guideline.builtin-names` | 不要用内置标识符名作为变量或参数名。 | `tool-golangci` | go.lint / predeclared, revive | ci | yes | 稳定检测。已通过 go.lint/golangci-lint 编排承接；报告按 linter 或 adapter 输出稳定 rule_id。 |
| `uber.guideline.defer-cleanup` | 获取资源后尽快 defer 释放，避免遗漏清理。 | `tool-golangci` | go.lint / bodyclose, revive | ci | yes | 资源 close 子集可稳定检测。已通过 go.lint/golangci-lint 编排承接；报告按 linter 或 adapter 输出稳定 rule_id。 |
| `uber.guideline.error-naming` | sentinel error 用 Err 前缀，error 类型用 Error 后缀。 | `tool-golangci` | go.lint / errname | ci | yes | Err 前缀 / Error 后缀。已通过 go.lint/golangci-lint 编排承接；报告按 linter 或 adapter 输出稳定 rule_id。 |
| `uber.guideline.error-wrapping` | 错误包装应使用 Go 1.13 的 %w、errors.Is/As 语义。 | `tool-golangci` | go.lint / errorlint | ci | yes | %w/Is/As。已通过 go.lint/golangci-lint 编排承接；报告按 linter 或 adapter 输出稳定 rule_id。 |
| `uber.guideline.mutable-globals` | 避免可变全局状态，降低测试和并发风险。 | `tool-golangci-config` | go.lint / gochecknoglobals | strict | yes | 例外清单必要。已通过 go.lint/golangci-lint 编排承接；报告按 linter 或 adapter 输出稳定 rule_id。 已从通用默认 lint 中移出，保留为 strict/团队可选规则。 |
| `uber.guideline.no-init` | 避免 init 中复杂逻辑，提升可测试性和可控性。 | `tool-golangci-config` | go.lint / gochecknoinits | strict | yes | registry/main/test 例外。已通过 go.lint/golangci-lint 编排承接；报告按 linter 或 adapter 输出稳定 rule_id。 已从通用默认 lint 中移出，保留为 strict/团队可选规则。 |
| `uber.guideline.no-panic` | 正常流程不要 panic。 | `tool-golangci-config` | go.lint / forbidigo, revive | strict | yes | 与官方合并。已通过 go.lint/golangci-lint 编排承接；报告按 linter 或 adapter 输出稳定 rule_id。 |
| `uber.guideline.pointer-to-interface` | 几乎不要使用 *interface，interface 应按值传递。 | `tool-golangci-config` | go.lint / revive, semantic | strict | yes | *interface 可稳定检测。已通过 go.lint/golangci-lint 编排承接；报告按 linter 或 adapter 输出稳定 rule_id。 |
| `uber.guideline.public-embedded-types` | 公开 struct 避免嵌入类型导致 API 泄漏。 | `tool-golangci-config` | go.lint / revive, semantic | strict | yes | public API 识别。已通过 go.lint/golangci-lint 编排承接；报告按 linter 或 adapter 输出稳定 rule_id。 |
| `uber.guideline.type-assertion-ok` | 类型断言应检查 ok，避免非预期 panic。 | `tool-golangci` | go.lint / forcetypeassert | ci | yes | 稳定检测。已通过 go.lint/golangci-lint 编排承接；报告按 linter 或 adapter 输出稳定 rule_id。 |
| `uber.linting.runners` | 使用统一 lint runner 确保本地和 CI 一致。 | `tool-golangci` | go.lint / go.lint | default | yes | 本项目通过 go.lint 编排。已通过 go.lint/golangci-lint 编排承接；报告按 linter 或 adapter 输出稳定 rule_id。 |
| `uber.pattern.parallel-tests` | 并行测试要正确处理 loop variable 和共享状态。 | `tool-golangci-config` | go.lint / paralleltest | strict | yes | 团队决定是否强制。已通过 go.lint/golangci-lint 编排承接；报告按 linter 或 adapter 输出稳定 rule_id。 已从通用默认 lint 中移出，保留为 strict/团队可选规则。 |
| `uber.perf.no-repeated-string-byte` | 避免在循环中重复 string/[]byte 转换。 | `tool-golangci-config` | go.lint / gocritic, perf linters | strict | yes | 性能 profile。已通过 go.lint/golangci-lint 编排承接；报告按 linter 或 adapter 输出稳定 rule_id。 |
| `uber.perf.strconv` | 简单类型转换优先 strconv，避免 fmt 的额外开销。 | `tool-golangci` | go.lint / gocritic, perfsprint | strict | yes | 性能 profile。已通过 go.lint/golangci-lint 编排承接；报告按 linter 或 adapter 输出稳定 rule_id。 |
| `uber.style.format-strings` | printf 格式字符串应尽量为常量，避免运行时格式错误。 | `tool-golangci-config` | go.lint / govet printf, revive | ci | yes | printf 风险。已通过 go.lint/golangci-lint 编排承接；报告按 linter 或 adapter 输出稳定 rule_id。 |
| `uber.style.group-declarations` | 相似声明放在一起提升阅读性。 | `tool-golangci` | go.lint / gofmt, gofumpt | ci | yes | 子集由格式化器覆盖。已通过 go.lint/golangci-lint 编排承接；报告按 linter 或 adapter 输出稳定 rule_id。 |
| `uber.style.import-order` | import 分组顺序应稳定。 | `tool-golangci` | go.lint / gci, goimports | default | yes | 与官方合并。已通过 go.lint/golangci-lint 编排承接；报告按 linter 或 adapter 输出稳定 rule_id。 |
| `uber.style.local-vars` | 局部变量声明应贴近使用点并利用零值。 | `tool-golangci` | go.lint / gofumpt, staticcheck | ci | yes | 零值 var、短声明子集。已通过 go.lint/golangci-lint 编排承接；报告按 linter 或 adapter 输出稳定 rule_id。 |
| `uber.style.maps` | map 初始化应表达是否为空、是否有初值和容量。 | `tool-golangci-config` | go.lint / makezero, prealloc | strict | yes | 容量和空 map。已通过 go.lint/golangci-lint 编排承接；报告按 linter 或 adapter 输出稳定 rule_id。 |
| `uber.style.package-names` | package 名应短小写且有意义。 | `tool-golangci-config` | go.lint / denylist, revive | strict | yes | 与官方合并。已通过 go.lint/golangci-lint 编排承接；报告按 linter 或 adapter 输出稳定 rule_id。 |
| `uber.style.printf-names` | 自定义 printf 风格函数命名/声明应能被 printf 检查识别。 | `tool-golangci-config` | go.lint / govet printf analyzer config | strict | yes | 自定义 printf 函数。已通过 go.lint/golangci-lint 编排承接；报告按 linter 或 adapter 输出稳定 rule_id。 |
| `uber.style.reduce-nesting` | 减少嵌套，让主路径更清楚。 | `tool-golangci-config` | go.lint / gocritic, revive | ci | yes | else-after-return 等。已通过 go.lint/golangci-lint 编排承接；报告按 linter 或 adapter 输出稳定 rule_id。 |
| `uber.style.struct-embedding` | struct 嵌入应谨慎，避免不清晰 API。 | `tool-golangci-config` | go.lint / revive, semantic | strict | yes | 与 public embedded 合并。已通过 go.lint/golangci-lint 编排承接；报告按 linter 或 adapter 输出稳定 rule_id。 |
| `uber.style.struct-field-names` | 初始化 struct 时字段名能提升可读性和抗变更能力。 | `tool-golangci` | go.lint / govet composites | ci | yes | 外部包 struct。已通过 go.lint/golangci-lint 编排承接；报告按 linter 或 adapter 输出稳定 rule_id。 |
| `uber.style.struct-references` | 初始化 struct 指针应保持清晰一致。 | `tool-golangci-config` | go.lint / gocritic, gofumpt | strict | yes | 可读性。已通过 go.lint/golangci-lint 编排承接；报告按 linter 或 adapter 输出稳定 rule_id。 |
| `uber.style.unnecessary-else` | return/break 后不需要 else。 | `tool-golangci` | go.lint / gocritic, revive | strict | yes | 稳定检测。已通过 go.lint/golangci-lint 编排承接；报告按 linter 或 adapter 输出稳定 rule_id。 已从通用默认 lint 中移出，保留为 strict/团队可选规则。 |
| `uber.style.zero-struct-var` | 零值 struct 用 var 声明更清楚。 | `tool-golangci-config` | go.lint / gofumpt, staticcheck | ci | yes | 机械子集。已通过 go.lint/golangci-lint 编排承接；报告按 linter 或 adapter 输出稳定 rule_id。 |

### B. 走自定义开发 / go.semantic

这些规则适合开发或扩展 `go.semantic` analyzer。只有能稳定从 AST、import、type info 或确定性控制流里判断的规则才放这里。

| Rule ID | 规则说明 | 处理方式 | 推荐承接 | 默认 | 已实现 | 备注 |
| --- | --- | --- | --- | --- | --- | --- |
| `go.official.import-blank` | blank import 只应用于明确 side effect 场景，并限制作用域。 | `tool-semantic` | go.semantic / import-blank | strict | yes | 内置 semantic 规则 import-blank 已实现；允许 main 包和 _test.go 例外，并有 analyzer 测试覆盖。 |
| `google.bp.no-tfatal-goroutine` | 不要在非测试 goroutine 中直接调用 t.Fatal。 | `tool-semantic` | go.semantic / no-tfatal-goroutine | strict | yes | 内置 semantic 规则 no-tfatal-goroutine 已实现；检测 go statement 内 t.Fatal/Fatalf/FailNow，并有 analyzer 测试覆盖。 |
| `google.imports.blank` | blank import 只用于明确副作用注册场景。 | `tool-semantic` | go.semantic / import-blank | strict | yes | 由 go.official.import-blank 同一内置 analyzer 覆盖语义；报告 rule_id 统一输出 go.official.import-blank。 |
| `google.libs.custom-contexts` | 不要自定义 context 类型或用自定义接口替代 context.Context。 | `tool-semantic` | go.semantic / custom-contexts | strict | yes | 内置 semantic 规则 custom-contexts 已实现；检测自定义 context-like interface，并有 analyzer 测试覆盖。 |
| `team.semantic.max-params` | 函数/方法入参个数不得超过配置阈值。 | `tool-semantic` | go.semantic / max-params | strict | yes | 配置使用该 catalog id 时，报告 rule_id 保持一致。 |
| `uber.guideline.channel-size` | channel buffer 通常为 0 或 1，更大容量需要明确理由。 | `tool-semantic` | go.semantic / channel-size | strict | yes | 内置 semantic 规则 channel-size 已实现；检测 make(chan T, N) 且 N>1，并有 analyzer 测试覆盖。 |
| `uber.guideline.enum-start-one` | iota enum 通常保留 0 作为 unknown/invalid，从 1 开始有效值。 | `tool-semantic` | go.semantic / enum-start-one | strict | yes | 内置 semantic 规则 enum-start-one 已实现；检测 iota enum 首项未预留 unknown/invalid zero，并有 analyzer 测试覆盖。 |
| `uber.guideline.exit-in-main` | 进程退出应集中在 main 层并只发生一次。 | `tool-semantic` | go.semantic / exit-in-main | strict | yes | 内置 semantic 规则 exit-in-main 已实现；检测非 package main main() 中的 os.Exit，并有 analyzer 测试覆盖。 |

### C. 走 LLM / 人工 review

这些规则需要上下文和设计判断。go-review 应把它们写入规范文档、LLM 报告和人工 checklist，但默认不作为确定性 gate。

| Rule ID | 规则说明 | 处理方式 | 推荐承接 | 默认 | 已实现 | 备注 |
| --- | --- | --- | --- | --- | --- | --- |
| `go.official.contexts` | context 使用要符合 Go 习惯，例如不存入 struct、不要自定义 Context、ctx 通常作为首参。 | `llm-review` | llm.review / AGENTS.md | doc | yes | context 包含多条子规则；只把能稳定拆出的子规则单独工具化。已写入 AGENTS.md 的 C 类 LLM review checklist；中文报告需携带 rule_id，默认不阻断。 |
| `go.official.copying` | 避免错误复制带锁或共享状态的值，slice/map 边界要考虑所有权。 | `llm-review` | llm.review / AGENTS.md | doc | yes | 锁、buffer、slice/map 所有权需上下文。已写入 AGENTS.md 的 C 类 LLM review checklist；中文报告需携带 rule_id，默认不阻断。 |
| `go.official.empty-slices` | 空 slice 通常用 nil slice 表达，除非 API/JSON 语义要求非 nil。 | `llm-review` | llm.review / AGENTS.md | doc | yes | API/JSON 语义可能需要非 nil slice，不做默认工具 gate。已写入 AGENTS.md 的 C 类 LLM review checklist；中文报告需携带 rule_id，默认不阻断。 |
| `go.official.goroutine-lifetimes` | 启动 goroutine 时要能说明退出条件和生命周期，避免泄漏。 | `llm-review` | llm.review / AGENTS.md | doc | yes | 生命周期语义很难稳定静态判断。已写入 AGENTS.md 的 C 类 LLM review checklist；中文报告需携带 rule_id，默认不阻断。 |
| `go.official.in-band-errors` | 不要用特殊返回值混在正常结果里表达错误，必要时返回 error。 | `llm-review` | llm.review / AGENTS.md | doc | yes | 是否要额外 error 返回值依赖 API 语义。已写入 AGENTS.md 的 C 类 LLM review checklist；中文报告需携带 rule_id，默认不阻断。 |
| `go.official.interfaces` | interface 通常由使用方定义，避免实现方过早暴露抽象。 | `llm-review` | llm.review / AGENTS.md | doc | yes | “接口由使用方定义”误判风险高。已写入 AGENTS.md 的 C 类 LLM review checklist；中文报告需携带 rule_id，默认不阻断。 |
| `go.official.line-length` | Go 不设固定行长，但长行应以可读性为准处理。 | `llm-review` | llm.review / AGENTS.md | doc | yes | Go 官方不设固定列宽。已写入 AGENTS.md 的 C 类 LLM review checklist；中文报告需携带 rule_id，默认不阻断。 |
| `go.official.pass-values` | 值传递还是指针传递应基于大小、可变性和语义选择。 | `llm-review` | llm.review / AGENTS.md | doc | yes | 值/指针传递取决于大小、可变性、接口。已写入 AGENTS.md 的 C 类 LLM review checklist；中文报告需携带 rule_id，默认不阻断。 |
| `go.official.receiver-names` | receiver 名应短、能代表类型，且同一类型保持一致。 | `llm-review` | llm.review / AGENTS.md | doc | yes | 单纯命名和一致性误判较多；具体一致性 analyzer 成熟后再工具化。已写入 AGENTS.md 的 C 类 LLM review checklist；中文报告需携带 rule_id，默认不阻断。 |
| `go.official.receiver-type` | 值 receiver 还是指针 receiver 应保持语义一致。 | `llm-review` | llm.review / AGENTS.md | doc | yes | 值/指针 receiver 取决于语义。已写入 AGENTS.md 的 C 类 LLM review checklist；中文报告需携带 rule_id，默认不阻断。 |
| `go.official.sync-functions` | API 通常应同步返回结果；异步行为要由调用方控制。 | `llm-review` | llm.review / AGENTS.md | doc | yes | API 是否异步取决于设计。已写入 AGENTS.md 的 C 类 LLM review checklist；中文报告需携带 rule_id，默认不阻断。 |
| `go.official.useful-test-failures` | 测试失败信息要能定位函数、输入、got/want 和差异。 | `llm-review` | llm.review / AGENTS.md | doc | yes | got/want、输入、函数名等可做弱检测。已写入 AGENTS.md 的 C 类 LLM review checklist；中文报告需携带 rule_id，默认不阻断。 |
| `go.test.compare-full-structures` | 优先比较完整结构，避免只断言部分字段漏掉回归。 | `llm-review` | llm.review / AGENTS.md | doc | yes | 可用 go-cmp 建议。已写入 AGENTS.md 的 C 类 LLM review checklist；中文报告需携带 rule_id，默认不阻断。 |
| `go.test.compare-stable-results` | 测试应比较稳定结果，避免依赖 map 顺序、时间等不稳定因素。 | `llm-review` | llm.review / AGENTS.md | doc | yes | 稳定性依赖被测对象。已写入 AGENTS.md 的 C 类 LLM review checklist；中文报告需携带 rule_id，默认不阻断。 |
| `go.test.equality-diffs` | 复杂对象比较失败时应输出有用 diff。 | `llm-review` | llm.review / AGENTS.md | doc | yes | 推荐 diff 信息，不适合硬 gate。已写入 AGENTS.md 的 C 类 LLM review checklist；中文报告需携带 rule_id，默认不阻断。 |
| `go.test.error-semantics` | 测试 error 应验证语义，例如 errors.Is/As，而不只比字符串。 | `llm-review` | llm.review / AGENTS.md | doc | yes | 错误语义比较需上下文。已写入 AGENTS.md 的 C 类 LLM review checklist；中文报告需携带 rule_id，默认不阻断。 |
| `go.test.got-before-want` | 测试输出和断言应保持 got 在前、want 在后。 | `llm-review` | llm.review / AGENTS.md | doc | yes | 只有部分断言库能稳定检测；默认宽松处理。已写入 AGENTS.md 的 C 类 LLM review checklist；中文报告需携带 rule_id，默认不阻断。 |
| `go.test.human-readable-subtests` | 子测试名称应让人能看懂场景，而不是机械索引。 | `llm-review` | llm.review / AGENTS.md | doc | yes | 静态硬判价值低。已写入 AGENTS.md 的 C 类 LLM review checklist；中文报告需携带 rule_id，默认不阻断。 |
| `go.test.identify-function` | 失败信息应能看出正在测试哪个函数或行为。 | `llm-review` | llm.review / AGENTS.md | doc | yes | 失败信息语义判断。已写入 AGENTS.md 的 C 类 LLM review checklist；中文报告需携带 rule_id，默认不阻断。 |
| `go.test.identify-input` | 失败信息应包含导致失败的关键输入。 | `llm-review` | llm.review / AGENTS.md | doc | yes | 失败信息语义判断。已写入 AGENTS.md 的 C 类 LLM review checklist；中文报告需携带 rule_id，默认不阻断。 |
| `go.test.keep-going` | 能继续收集多个失败时用 t.Error；无法继续时才 t.Fatal。 | `llm-review` | llm.review / AGENTS.md | doc | yes | t.Error/t.Fatal 取舍依赖上下文。已写入 AGENTS.md 的 C 类 LLM review checklist；中文报告需携带 rule_id，默认不阻断。 |
| `go.test.print-diffs` | 复杂比较失败时打印 diff，而不只是 true/false。 | `llm-review` | llm.review / AGENTS.md | doc | yes | 推荐 cmp.Diff 等。已写入 AGENTS.md 的 C 类 LLM review checklist；中文报告需携带 rule_id，默认不阻断。 |
| `go.test.table-driven-vs-multiple` | 表驱动和多个测试函数应按可读性选择，不盲目套表。 | `llm-review` | llm.review / AGENTS.md | doc | yes | 测试结构设计判断。已写入 AGENTS.md 的 C 类 LLM review checklist；中文报告需携带 rule_id，默认不阻断。 |
| `google.bp.add-error-info` | 返回错误时应添加有用上下文。 | `llm-review` | llm.review / AGENTS.md | doc | yes | 错误上下文质量。已写入 AGENTS.md 的 C 类 LLM review checklist；中文报告需携带 rule_id，默认不阻断。 |
| `google.bp.avoid-repetition` | 命名和 API 应避免重复已经由上下文表达的信息。 | `llm-review` | llm.review / AGENTS.md | doc | yes | 与 naming repetition 合并。已写入 AGENTS.md 的 C 类 LLM review checklist；中文报告需携带 rule_id，默认不阻断。 |
| `google.bp.checks-panics` | 程序不变量检查和 panic 应限制在不可恢复场景。 | `llm-review` | llm.review / AGENTS.md | doc | yes | panic 是否合理依赖语义；禁用特定调用可另建 tool 规则。已写入 AGENTS.md 的 C 类 LLM review checklist；中文报告需携带 rule_id，默认不阻断。 |
| `google.bp.cli-complex` | 复杂 CLI 应有清晰子命令、flag 和错误体验。 | `llm-review` | llm.review / AGENTS.md | doc | yes | 产品/API 设计。已写入 AGENTS.md 的 C 类 LLM review checklist；中文报告需携带 rule_id，默认不阻断。 |
| `google.bp.constant-strings` | 重复使用或有语义的字符串应考虑常量。 | `llm-review` | llm.review / AGENTS.md | doc | yes | 是否抽常量依赖语义和重复成本。已写入 AGENTS.md 的 C 类 LLM review checklist；中文报告需携带 rule_id，默认不阻断。 |
| `google.bp.default-instance` | 默认实例 API 应避免隐藏状态和测试困难。 | `llm-review` | llm.review / AGENTS.md | doc | yes | API 设计。已写入 AGENTS.md 的 C 类 LLM review checklist；中文报告需携带 rule_id，默认不阻断。 |
| `google.bp.doc-cleanup` | 需要调用方清理的资源必须在文档中说明。 | `llm-review` | llm.review / AGENTS.md | doc | yes | 文档质量。已写入 AGENTS.md 的 C 类 LLM review checklist；中文报告需携带 rule_id，默认不阻断。 |
| `google.bp.doc-concurrency` | 并发安全性和调用约束应写清楚。 | `llm-review` | llm.review / AGENTS.md | doc | yes | 文档质量。已写入 AGENTS.md 的 C 类 LLM review checklist；中文报告需携带 rule_id，默认不阻断。 |
| `google.bp.doc-contexts` | 涉及 context 的 API 文档应说明取消、超时和值语义。 | `llm-review` | llm.review / AGENTS.md | doc | yes | 文档质量。已写入 AGENTS.md 的 C 类 LLM review checklist；中文报告需携带 rule_id，默认不阻断。 |
| `google.bp.doc-conventions` | 文档应说明行为、参数、并发、安全和错误语义。 | `llm-review` | llm.review / AGENTS.md | doc | yes | 语义完整性不作为确定性工具结论。已写入 AGENTS.md 的 C 类 LLM review checklist；中文报告需携带 rule_id，默认不阻断。 |
| `google.bp.doc-errors` | 公开 API 应说明可能返回的关键错误语义。 | `llm-review` | llm.review / AGENTS.md | doc | yes | 文档质量。已写入 AGENTS.md 的 C 类 LLM review checklist；中文报告需携带 rule_id，默认不阻断。 |
| `google.bp.doc-params-config` | 参数和配置文档应解释默认值、范围和影响。 | `llm-review` | llm.review / AGENTS.md | doc | yes | 文档质量。已写入 AGENTS.md 的 C 类 LLM review checklist；中文报告需携带 rule_id，默认不阻断。 |
| `google.bp.effective-interfaces` | 接口应小、稳定、表达行为。 | `llm-review` | llm.review / AGENTS.md | doc | yes | 设计判断。已写入 AGENTS.md 的 C 类 LLM review checklist；中文报告需携带 rule_id，默认不阻断。 |
| `google.bp.error-structure` | 错误结构应便于调用方判断和处理。 | `llm-review` | llm.review / AGENTS.md | doc | yes | API 语义。已写入 AGENTS.md 的 C 类 LLM review checklist；中文报告需携带 rule_id，默认不阻断。 |
| `google.bp.function-argument-lists` | 参数列表过长时考虑配置对象或选项结构。 | `llm-review` | llm.review / AGENTS.md | doc | yes | “参数个数”可检测，但“是否应改成配置对象”不是稳定工具结论。已写入 AGENTS.md 的 C 类 LLM review checklist；中文报告需携带 rule_id，默认不阻断。 |
| `google.bp.function-method-names` | 函数和方法名应表达行为，避免重复包/类型上下文。 | `llm-review` | llm.review / AGENTS.md | doc | yes | 语义命名。已写入 AGENTS.md 的 C 类 LLM review checklist；中文报告需携带 rule_id，默认不阻断。 |
| `google.bp.global-state` | 全局状态应最小化并保持可测试。 | `llm-review` | llm.review / AGENTS.md | doc | yes | 设计原则不等同于确定性工具覆盖。已写入 AGENTS.md 的 C 类 LLM review checklist；中文报告需携带 rule_id，默认不阻断。 |
| `google.bp.import-protobuf` | protobuf 消息和 stub 的导入/使用应保持清晰边界。 | `llm-review` | llm.review / AGENTS.md | doc | yes | 依赖具体 protobuf 组织。已写入 AGENTS.md 的 C 类 LLM review checklist；中文报告需携带 rule_id，默认不阻断。 |
| `google.bp.interface-ownership` | 接口归属和可见性应服务调用方。 | `llm-review` | llm.review / AGENTS.md | doc | yes | 语义判断。已写入 AGENTS.md 的 C 类 LLM review checklist；中文报告需携带 rule_id，默认不阻断。 |
| `google.bp.logging-errors` | 不要既记录错误又原样返回导致重复日志。 | `llm-review` | llm.review / AGENTS.md | doc | yes | 避免重复处理，需上下文。已写入 AGENTS.md 的 C 类 LLM review checklist；中文报告需携带 rule_id，默认不阻断。 |
| `google.bp.option-structure` | 多个可选参数可用 options/config 结构表达。 | `llm-review` | llm.review / AGENTS.md | doc | yes | API 设计。已写入 AGENTS.md 的 C 类 LLM review checklist；中文报告需携带 rule_id，默认不阻断。 |
| `google.bp.package-size` | 包大小应服务内聚性，过大时考虑拆分。 | `llm-review` | llm.review / AGENTS.md | doc | yes | 阈值项目化。已写入 AGENTS.md 的 C 类 LLM review checklist；中文报告需携带 rule_id，默认不阻断。 |
| `google.bp.program-init` | 初始化流程应显式、可测试、失败清晰。 | `llm-review` | llm.review / AGENTS.md | doc | yes | 初始化策略。已写入 AGENTS.md 的 C 类 LLM review checklist；中文报告需携带 rule_id，默认不阻断。 |
| `google.bp.real-transports` | 集成测试尽量使用真实传输层以减少 mock 偏差。 | `llm-review` | llm.review / AGENTS.md | doc | yes | 测试策略。已写入 AGENTS.md 的 C 类 LLM review checklist；中文报告需携带 rule_id，默认不阻断。 |
| `google.bp.scoped-test-setup` | 测试 setup 应尽量局部化，避免共享状态污染。 | `llm-review` | llm.review / AGENTS.md | doc | yes | 测试组织。已写入 AGENTS.md 的 C 类 LLM review checklist；中文报告需携带 rule_id，默认不阻断。 |
| `google.bp.sentinel-placement` | sentinel error 的定义位置应服务 API 使用者。 | `llm-review` | llm.review / AGENTS.md | doc | yes | API 设计。已写入 AGENTS.md 的 C 类 LLM review checklist；中文报告需携带 rule_id，默认不阻断。 |
| `google.bp.string-plus` | 简单字符串拼接用 + 更直接。 | `llm-review` | llm.review / AGENTS.md | doc | yes | 性能/可读性判断。已写入 AGENTS.md 的 C 类 LLM review checklist；中文报告需携带 rule_id，默认不阻断。 |
| `google.bp.string-sprintf` | 需要格式化多个值时用 fmt.Sprintf。 | `llm-review` | llm.review / AGENTS.md | doc | yes | 可读性判断。已写入 AGENTS.md 的 C 类 LLM review checklist；中文报告需携带 rule_id，默认不阻断。 |
| `google.bp.t-error-vs-fatal` | t.Error 和 t.Fatal 的选择取决于失败后是否还能继续。 | `llm-review` | llm.review / AGENTS.md | doc | yes | 语义判断。已写入 AGENTS.md 的 C 类 LLM review checklist；中文报告需携带 rule_id，默认不阻断。 |
| `google.bp.test-double-packages` | 测试替身和 helper 包应简单、局部、避免过度框架化。 | `llm-review` | llm.review / AGENTS.md | doc | yes | 测试架构。已写入 AGENTS.md 的 C 类 LLM review checklist；中文报告需携带 rule_id，默认不阻断。 |
| `google.bp.test-helper-errors` | 测试 helper 中处理错误要保留调用位置和上下文。 | `llm-review` | llm.review / AGENTS.md | doc | yes | 测试 helper 质量。已写入 AGENTS.md 的 C 类 LLM review checklist；中文报告需携带 rule_id，默认不阻断。 |
| `google.bp.test-in-test` | 断言和失败控制应尽量留在 Test 函数层。 | `llm-review` | llm.review / AGENTS.md | doc | yes | helper 设计。已写入 AGENTS.md 的 C 类 LLM review checklist；中文报告需携带 rule_id，默认不阻断。 |
| `google.bp.testmain` | 只有确有全局测试生命周期需求时才用 TestMain。 | `llm-review` | llm.review / AGENTS.md | doc | yes | 是否需要 TestMain 依赖策略。已写入 AGENTS.md 的 C 类 LLM review checklist；中文报告需携带 rule_id，默认不阻断。 |
| `google.bp.unnecessary-interfaces` | 不要为了测试或习惯过早抽象接口。 | `llm-review` | llm.review / AGENTS.md | doc | yes | 语义判断。已写入 AGENTS.md 的 C 类 LLM review checklist；中文报告需携带 rule_id，默认不阻断。 |
| `google.bp.validation-apis` | 测试验证 helper 应可扩展且失败信息清晰。 | `llm-review` | llm.review / AGENTS.md | doc | yes | 测试 API 设计。已写入 AGENTS.md 的 C 类 LLM review checklist；中文报告需携带 rule_id，默认不阻断。 |
| `google.bp.variadic-options` | 可变 options 适合扩展性 API，但不应过度使用。 | `llm-review` | llm.review / AGENTS.md | doc | yes | API 设计。已写入 AGENTS.md 的 C 类 LLM review checklist；中文报告需携带 rule_id，默认不阻断。 |
| `google.bp.when-to-panic` | 只有真正不可恢复或编程错误场景才 panic。 | `llm-review` | llm.review / AGENTS.md | doc | yes | 语义判断。已写入 AGENTS.md 的 C 类 LLM review checklist；中文报告需携带 rule_id，默认不阻断。 |
| `google.commentary.comment-line-length` | 注释行没有硬性长度限制，但应保持阅读舒适。 | `llm-review` | llm.review / AGENTS.md | doc | yes | 不做固定列宽。已写入 AGENTS.md 的 C 类 LLM review checklist；中文报告需携带 rule_id，默认不阻断。 |
| `google.commentary.doc-comments` | 文档注释应面向 API 使用者解释用途和行为。 | `llm-review` | llm.review / AGENTS.md | doc | yes | 内容质量不是稳定机器判断；存在性可另作为工具子规则。已写入 AGENTS.md 的 C 类 LLM review checklist；中文报告需携带 rule_id，默认不阻断。 |
| `google.commentary.package-comments` | package 注释应解释包的整体职责。 | `llm-review` | llm.review / AGENTS.md | doc | yes | 内容质量默认不硬 gate。已写入 AGENTS.md 的 C 类 LLM review checklist；中文报告需携带 rule_id，默认不阻断。 |
| `google.errors.in-band-errors` | 不要用特殊正常返回值来表达错误语义。 | `llm-review` | llm.review / AGENTS.md | doc | yes | 与官方合并。已写入 AGENTS.md 的 C 类 LLM review checklist；中文报告需携带 rule_id，默认不阻断。 |
| `google.errors.returning-errors` | 函数是否返回 error 应基于调用方能否处理失败。 | `llm-review` | llm.review / AGENTS.md | doc | yes | API 是否返回 error 依赖语义。已写入 AGENTS.md 的 C 类 LLM review checklist；中文报告需携带 rule_id，默认不阻断。 |
| `google.language.copying` | 复制带锁或共享状态的值前要理解所有权和副作用。 | `llm-review` | llm.review / AGENTS.md | doc | yes | 与官方 copying 合并。已写入 AGENTS.md 的 C 类 LLM review checklist；中文报告需携带 rule_id，默认不阻断。 |
| `google.language.generics` | 泛型应降低重复或提升类型安全，不应增加无谓复杂度。 | `llm-review` | llm.review / AGENTS.md | doc | yes | 是否过度泛型化依赖设计。已写入 AGENTS.md 的 C 类 LLM review checklist；中文报告需携带 rule_id，默认不阻断。 |
| `google.language.goroutine-lifetimes` | goroutine 应有明确退出条件和生命周期归属。 | `llm-review` | llm.review / AGENTS.md | doc | yes | 与官方合并。已写入 AGENTS.md 的 C 类 LLM review checklist；中文报告需携带 rule_id，默认不阻断。 |
| `google.language.interfaces` | 接口应在真正需要抽象时引入，通常由使用方拥有。 | `llm-review` | llm.review / AGENTS.md | doc | yes | 与官方合并。已写入 AGENTS.md 的 C 类 LLM review checklist；中文报告需携带 rule_id，默认不阻断。 |
| `google.language.must-functions` | Must 风格函数只适合初始化或测试等失败即不可恢复场景。 | `llm-review` | llm.review / AGENTS.md | doc | yes | 是否允许 Must 依赖 API 设计。已写入 AGENTS.md 的 C 类 LLM review checklist；中文报告需携带 rule_id，默认不阻断。 |
| `google.language.nil-slices` | 空 slice 通常可用 nil slice，但 API 语义可能例外。 | `llm-review` | llm.review / AGENTS.md | doc | yes | 与官方 empty slices 合并，默认不硬 gate。已写入 AGENTS.md 的 C 类 LLM review checklist；中文报告需携带 rule_id，默认不阻断。 |
| `google.language.pass-values` | 值/指针传递取决于大小、可变性和语义。 | `llm-review` | llm.review / AGENTS.md | doc | yes | 与官方合并。已写入 AGENTS.md 的 C 类 LLM review checklist；中文报告需携带 rule_id，默认不阻断。 |
| `google.language.receiver-type` | receiver 用值还是指针应保持语义一致。 | `llm-review` | llm.review / AGENTS.md | doc | yes | 与官方合并。已写入 AGENTS.md 的 C 类 LLM review checklist；中文报告需携带 rule_id，默认不阻断。 |
| `google.language.sync-functions` | API 默认应同步完成；异步行为要由调用方清晰控制。 | `llm-review` | llm.review / AGENTS.md | doc | yes | 与官方合并。已写入 AGENTS.md 的 C 类 LLM review checklist；中文报告需携带 rule_id，默认不阻断。 |
| `google.language.type-aliases` | type alias 主要用于迁移/兼容，不应滥用。 | `llm-review` | llm.review / AGENTS.md | doc | yes | 是否需要 alias 依赖迁移/兼容。已写入 AGENTS.md 的 C 类 LLM review checklist；中文报告需携带 rule_id，默认不阻断。 |
| `google.language.use-q` | 需要清楚展示字符串内容时优先用 %q。 | `llm-review` | llm.review / AGENTS.md | doc | yes | 错误/日志可读性。已写入 AGENTS.md 的 C 类 LLM review checklist；中文报告需携带 rule_id，默认不阻断。 |
| `google.language.zero-value-fields` | 结构体字面量中是否省略零值字段应服务可读性。 | `llm-review` | llm.review / AGENTS.md | doc | yes | 可读性和显式性取舍较主观，默认宽松处理。已写入 AGENTS.md 的 C 类 LLM review checklist；中文报告需携带 rule_id，默认不阻断。 |
| `google.libs.contexts` | context 只用于请求范围取消、超时和元数据传播。 | `llm-review` | llm.review / AGENTS.md | doc | yes | context 包含多条子规则；只把能稳定拆出的子规则单独工具化。已写入 AGENTS.md 的 C 类 LLM review checklist；中文报告需携带 rule_id，默认不阻断。 |
| `google.libs.flags` | 命令行 flag 设计应清晰、稳定、可组合。 | `llm-review` | llm.review / AGENTS.md | doc | yes | CLI 设计语义。已写入 AGENTS.md 的 C 类 LLM review checklist；中文报告需携带 rule_id，默认不阻断。 |
| `google.libs.logging` | 日志策略应一致，避免混乱级别和重复记录错误。 | `llm-review` | llm.review / AGENTS.md | doc | yes | 项目日志策略。已写入 AGENTS.md 的 C 类 LLM review checklist；中文报告需携带 rule_id，默认不阻断。 |
| `google.naming.receiver-names` | receiver 名应短且同类型一致。 | `llm-review` | llm.review / AGENTS.md | doc | yes | 与官方 receiver 规则合并，默认不硬 gate。已写入 AGENTS.md 的 C 类 LLM review checklist；中文报告需携带 rule_id，默认不阻断。 |
| `google.naming.repetition` | 避免包名、类型名、变量名重复上下文。 | `llm-review` | llm.review / AGENTS.md | doc | yes | 包名/类型名/变量名重复语义。已写入 AGENTS.md 的 C 类 LLM review checklist；中文报告需携带 rule_id，默认不阻断。 |
| `google.test.data-driven-cases` | 数据驱动用例应把输入、期望和名称表达清楚。 | `llm-review` | llm.review / AGENTS.md | doc | yes | 用例结构。已写入 AGENTS.md 的 C 类 LLM review checklist；中文报告需携带 rule_id，默认不阻断。 |
| `google.test.equality-diffs` | 相等比较失败时应提供可诊断 diff。 | `llm-review` | llm.review / AGENTS.md | doc | yes | diff 质量。已写入 AGENTS.md 的 C 类 LLM review checklist；中文报告需携带 rule_id，默认不阻断。 |
| `google.test.error-semantics` | 测试错误时应验证语义而非脆弱字符串。 | `llm-review` | llm.review / AGENTS.md | doc | yes | 错误语义比较。已写入 AGENTS.md 的 C 类 LLM review checklist；中文报告需携带 rule_id，默认不阻断。 |
| `google.test.full-structure` | 复杂对象优先比较完整结构，避免遗漏字段。 | `llm-review` | llm.review / AGENTS.md | doc | yes | 推荐 cmp/diff。已写入 AGENTS.md 的 C 类 LLM review checklist；中文报告需携带 rule_id，默认不阻断。 |
| `google.test.identify-function` | 测试失败信息应说明正在验证哪个函数/行为。 | `llm-review` | llm.review / AGENTS.md | doc | yes | 失败信息质量。已写入 AGENTS.md 的 C 类 LLM review checklist；中文报告需携带 rule_id，默认不阻断。 |
| `google.test.identify-input` | 测试失败信息应包含关键输入。 | `llm-review` | llm.review / AGENTS.md | doc | yes | 失败信息质量。已写入 AGENTS.md 的 C 类 LLM review checklist；中文报告需携带 rule_id，默认不阻断。 |
| `google.test.identifying-row` | 表驱动失败应能定位具体 case。 | `llm-review` | llm.review / AGENTS.md | doc | yes | case 名。已写入 AGENTS.md 的 C 类 LLM review checklist；中文报告需携带 rule_id，默认不阻断。 |
| `google.test.keep-going` | 能继续收集失败时不要过早 Fatal。 | `llm-review` | llm.review / AGENTS.md | doc | yes | t.Error/t.Fatal 取舍。已写入 AGENTS.md 的 C 类 LLM review checklist；中文报告需携带 rule_id，默认不阻断。 |
| `google.test.level-of-detail` | 测试失败信息要有足够细节但不过度噪音。 | `llm-review` | llm.review / AGENTS.md | doc | yes | 失败信息粒度。已写入 AGENTS.md 的 C 类 LLM review checklist；中文报告需携带 rule_id，默认不阻断。 |
| `google.test.package-external` | 外部包测试适合验证公开 API。 | `llm-review` | llm.review / AGENTS.md | doc | yes | 同上。已写入 AGENTS.md 的 C 类 LLM review checklist；中文报告需携带 rule_id，默认不阻断。 |
| `google.test.package-same` | 是否同包测试取决于是否需要访问内部实现。 | `llm-review` | llm.review / AGENTS.md | doc | yes | 测试包选择依赖目标。已写入 AGENTS.md 的 C 类 LLM review checklist；中文报告需携带 rule_id，默认不阻断。 |
| `google.test.print-diffs` | 复杂差异应打印 diff。 | `llm-review` | llm.review / AGENTS.md | doc | yes | 与 Go Test Comments 合并。已写入 AGENTS.md 的 C 类 LLM review checklist；中文报告需携带 rule_id，默认不阻断。 |
| `google.test.stable-results` | 测试只比较稳定结果，避免顺序/时间等不稳定因素。 | `llm-review` | llm.review / AGENTS.md | doc | yes | 稳定性语义。已写入 AGENTS.md 的 C 类 LLM review checklist；中文报告需携带 rule_id，默认不阻断。 |
| `google.test.subtest-names` | 子测试名称应可读并描述场景。 | `llm-review` | llm.review / AGENTS.md | doc | yes | 名称可读性。已写入 AGENTS.md 的 C 类 LLM review checklist；中文报告需携带 rule_id，默认不阻断。 |
| `google.test.subtests` | 子测试应让测试场景结构更清楚。 | `llm-review` | llm.review / AGENTS.md | doc | yes | 测试结构判断。已写入 AGENTS.md 的 C 类 LLM review checklist；中文报告需携带 rule_id，默认不阻断。 |
| `google.test.table-driven` | 表驱动测试应提升清晰度，而不是制造复杂表格。 | `llm-review` | llm.review / AGENTS.md | doc | yes | 表驱动是否合适。已写入 AGENTS.md 的 C 类 LLM review checklist；中文报告需携带 rule_id，默认不阻断。 |
| `team.observability.no-silent-drop` | debug、audit、trace、event、metrics、telemetry 等非阻塞观测写入通常不应影响主业务流程；如果调用返回 error，应优先记录降级日志、指标或使用项目已有安全封装，避免完全静默丢弃。只有项目明确允许忽略时，才应显式说明原因。 | `llm-review` | llm.review / AGENTS.md | review | yes | 已写入 AGENTS.md 的 C 类 LLM review checklist；LLM 修复时不应把观测失败升级为主流程失败；优先复用 recordDebugEvent、logIgnoredError、observeError 等项目既有封装；高频路径要避免日志风暴。 |
| `uber.guideline.copy-boundaries` | slice/map 穿越 API 边界时要考虑拷贝，避免外部修改内部状态。 | `llm-review` | llm.review / AGENTS.md | doc | yes | 所有权语义。已写入 AGENTS.md 的 C 类 LLM review checklist；中文报告需携带 rule_id，默认不阻断。 |
| `uber.guideline.error-types` | 错误类型设计应服务调用方匹配和处理。 | `llm-review` | llm.review / AGENTS.md | doc | yes | 错误 API 设计。已写入 AGENTS.md 的 C 类 LLM review checklist；中文报告需携带 rule_id，默认不阻断。 |
| `uber.guideline.field-tags` | 会序列化的 struct 字段应有明确 tag。 | `llm-review` | llm.review / AGENTS.md | doc | yes | 序列化语义。已写入 AGENTS.md 的 C 类 LLM review checklist；中文报告需携带 rule_id，默认不阻断。 |
| `uber.guideline.handle-errors-once` | error 应只处理一次，避免 log 后再返回造成重复处理。 | `llm-review` | llm.review / AGENTS.md | doc | yes | log/return 语义。已写入 AGENTS.md 的 C 类 LLM review checklist；中文报告需携带 rule_id，默认不阻断。 |
| `uber.guideline.interface-compliance` | 关键类型可用编译期断言证明实现了接口。 | `llm-review` | llm.review / AGENTS.md | doc | yes | 是否需要断言依赖 API。已写入 AGENTS.md 的 C 类 LLM review checklist；中文报告需携带 rule_id，默认不阻断。 |
| `uber.guideline.no-fire-forget` | 不要启动无人等待、无退出条件的 goroutine。 | `llm-review` | llm.review / AGENTS.md | doc | yes | 生命周期语义。已写入 AGENTS.md 的 C 类 LLM review checklist；中文报告需携带 rule_id，默认不阻断。 |
| `uber.guideline.receivers-interfaces` | receiver 选择会影响方法集和接口实现，应一致。 | `llm-review` | llm.review / AGENTS.md | doc | yes | receiver/interface 语义。已写入 AGENTS.md 的 C 类 LLM review checklist；中文报告需携带 rule_id，默认不阻断。 |
| `uber.guideline.use-time` | 时间点/时长应使用 time.Time/time.Duration，而不是裸 int/string。 | `llm-review` | llm.review / AGENTS.md | doc | yes | API/schema 语义。已写入 AGENTS.md 的 C 类 LLM review checklist；中文报告需携带 rule_id，默认不阻断。 |
| `uber.pattern.functional-options` | 可选参数很多时可考虑 functional options，但不是默认选择。 | `llm-review` | llm.review / AGENTS.md | doc | yes | API 模式建议，不是硬规则。已写入 AGENTS.md 的 C 类 LLM review checklist；中文报告需携带 rule_id，默认不阻断。 |
| `uber.pattern.table-complexity` | 不要把简单测试做成复杂表驱动框架。 | `llm-review` | llm.review / AGENTS.md | doc | yes | 结构判断。已写入 AGENTS.md 的 C 类 LLM review checklist；中文报告需携带 rule_id，默认不阻断。 |
| `uber.pattern.test-tables` | 表驱动测试应简单、命名清晰、错误信息可诊断。 | `llm-review` | llm.review / AGENTS.md | doc | yes | 表驱动结构。已写入 AGENTS.md 的 C 类 LLM review checklist；中文报告需携带 rule_id，默认不阻断。 |
| `uber.style.consistency` | 同一项目内相似代码应保持一致。 | `llm-review` | llm.review / AGENTS.md | doc | yes | 项目一致性。已写入 AGENTS.md 的 C 类 LLM review checklist；中文报告需携带 rule_id，默认不阻断。 |
| `uber.style.function-names` | 函数名应表达动作和结果。 | `llm-review` | llm.review / AGENTS.md | doc | yes | 语义命名。已写入 AGENTS.md 的 C 类 LLM review checklist；中文报告需携带 rule_id，默认不阻断。 |
| `uber.style.function-ordering` | 文件内函数顺序应帮助阅读。 | `llm-review` | llm.review / AGENTS.md | doc | yes | 文件组织主观性高。已写入 AGENTS.md 的 C 类 LLM review checklist；中文报告需携带 rule_id，默认不阻断。 |
| `uber.style.import-aliasing` | import alias 只在必要时使用。 | `llm-review` | llm.review / AGENTS.md | doc | yes | “必要性”依赖上下文，默认不硬 gate。已写入 AGENTS.md 的 C 类 LLM review checklist；中文报告需携带 rule_id，默认不阻断。 |
| `uber.style.long-lines` | 避免影响阅读的超长行，但不设机械列宽。 | `llm-review` | llm.review / AGENTS.md | doc | yes | 不建议固定列宽。已写入 AGENTS.md 的 C 类 LLM review checklist；中文报告需携带 rule_id，默认不阻断。 |
| `uber.style.naked-parameters` | 避免多个裸 bool/int 参数让调用点难读。 | `llm-review` | llm.review / AGENTS.md | doc | yes | bool/int 参数语义。已写入 AGENTS.md 的 C 类 LLM review checklist；中文报告需携带 rule_id，默认不阻断。 |
| `uber.style.nil-valid-slice` | nil slice 是有效空 slice，通常不必构造空 literal。 | `llm-review` | llm.review / AGENTS.md | doc | yes | 与官方 empty slices 合并，默认不硬 gate。已写入 AGENTS.md 的 C 类 LLM review checklist；中文报告需携带 rule_id，默认不阻断。 |
| `uber.style.omit-zero-fields` | struct literal 中可省略无意义零值字段。 | `llm-review` | llm.review / AGENTS.md | doc | yes | 显式零值有时更清楚，默认宽松处理。已写入 AGENTS.md 的 C 类 LLM review checklist；中文报告需携带 rule_id，默认不阻断。 |
| `uber.style.raw-strings` | 复杂字符串优先用 raw string 降低转义噪音。 | `llm-review` | llm.review / AGENTS.md | doc | yes | 可读性判断，不默认工具 gate。已写入 AGENTS.md 的 C 类 LLM review checklist；中文报告需携带 rule_id，默认不阻断。 |
| `uber.style.reduce-scope` | 变量作用域应尽量小。 | `llm-review` | llm.review / AGENTS.md | doc | yes | 作用域是否过大通常依赖可读性上下文。已写入 AGENTS.md 的 C 类 LLM review checklist；中文报告需携带 rule_id，默认不阻断。 |
| `uber.style.top-level-vars` | 顶层变量声明应清晰分组。 | `llm-review` | llm.review / AGENTS.md | doc | yes | 组织风格。已写入 AGENTS.md 的 C 类 LLM review checklist；中文报告需携带 rule_id，默认不阻断。 |
| `uber.style.unexported-global-prefix` | 可用前缀标识未导出的全局变量以提示风险。 | `llm-review` | llm.review / AGENTS.md | doc | yes | Uber 风格项，是否采用由团队决定。已写入 AGENTS.md 的 C 类 LLM review checklist；中文报告需携带 rule_id，默认不阻断。 |

### D. Candidate：待确认后再归类

这些规则暂不标实现。下一步要么证明能由 lint/semantic 稳定处理并迁入 A/B，要么降级为 LLM review。

| Rule ID | 规则说明 | 处理方式 | 推荐承接 | 默认 | 已实现 | 备注 |
| --- | --- | --- | --- | --- | --- | --- |
| `go.official.variable-names` | 变量名长度和信息量应匹配作用域，短作用域可短名。 | `candidate` | varnamelen | strict | no | 阈值和例外高度项目化，未确认前不标为工具覆盖。 |
| `google.bp.channel-direction` | 函数参数中的 channel 能限制方向时应限制方向。 | `candidate` | 候选待确认 | strict | no | 需先证明只读/只写推断稳定。 |
| `google.bp.shadowing` | 避免变量遮蔽造成读错值或误用。 | `candidate` | copyloopvar, shadow | ci | no | 具体 linter 与误判率需按 golangci-lint 版本确认。 |
| `google.bp.size-hints` | 已知容量时可给 slice/map 容量提示。 | `candidate` | prealloc | strict | no | 容量是否“已知”有误判成本，先候选。 |
| `google.language.use-any` | Go 1.18+ 中用 any 表达 interface{} 的别名语义。 | `candidate` | 候选待确认 | strict | no | 需要确认项目 Go 版本和 linter 支持后启用。 |
| `google.naming.variable-names` | 变量名应根据作用域选择信息量。 | `candidate` | 候选待确认 | strict | no | 需要团队阈值和误判评估后才能工具化。 |
| `uber.guideline.zero-value-mutex` | sync.Mutex/RWMutex 零值可用，通常不要 new 一个 mutex。 | `candidate` | 候选待确认 | strict | no | 先作为候选，避免误伤需要共享锁指针的特殊场景。 |
| `uber.perf.container-capacity` | 已知大小时给 slice/map 指定容量。 | `candidate` | makezero, prealloc | strict | no | 需要按项目误判率确认。 |
