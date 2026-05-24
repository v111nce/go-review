# AGENTS.md — Go Review LLM 规则执行指南

本文件承接 `rules/go-rules.json` 中 `handling: llm-review` 的 C 类规则。C 类规则需要设计意图、API 语义、并发/测试上下文或项目约定判断，不能当作确定性工具 gate。

## 执行原则

- 报告必须使用中文。
- 每条 LLM review 结论必须带稳定 `rule_id`，并能回到 `rules/go-rules.json` 查询来源。
- 不要把 C 类结论描述成 gofmt、golangci-lint、go test 或 go.semantic 的工具检测结果。
- C 类默认不阻断；只有用户或项目策略明确要求时，才升级为阻断项。
- 区分“证据”和“判断”：能引用文件/函数/调用链时给出位置；无法证明时标为建议或待确认。
- D 类 `candidate` 暂不执行、不阻断；等 catalog 重新归类后再处理。

## 输出格式

每个发现使用以下结构：

```text
- rule_id: <catalog rule id>
  位置: <file:line 或 相关文件范围>
  证据: <代码事实/调用关系/测试行为>
  判断: <为什么违反或可能违反该规则>
  风险: <可维护性/API/并发/测试诊断等影响>
  建议: <可执行修改方向>
  阻断: 默认否；若需要阻断必须说明项目策略依据
```

## C 类规则索引

共 121 条，按来源分组。详细字段以 `rules/go-rules.json` 为准。

### Go Code Review Comments（12）

- `go.official.contexts`：context 使用要符合 Go 习惯，例如不存入 struct、不要自定义 Context、ctx 通常作为首参。
- `go.official.copying`：避免错误复制带锁或共享状态的值，slice/map 边界要考虑所有权。
- `go.official.empty-slices`：空 slice 通常用 nil slice 表达，除非 API/JSON 语义要求非 nil。
- `go.official.goroutine-lifetimes`：启动 goroutine 时要能说明退出条件和生命周期，避免泄漏。
- `go.official.in-band-errors`：不要用特殊返回值混在正常结果里表达错误，必要时返回 error。
- `go.official.interfaces`：interface 通常由使用方定义，避免实现方过早暴露抽象。
- `go.official.line-length`：Go 不设固定行长，但长行应以可读性为准处理。
- `go.official.pass-values`：值传递还是指针传递应基于大小、可变性和语义选择。
- `go.official.receiver-names`：receiver 名应短、能代表类型，且同一类型保持一致。
- `go.official.receiver-type`：值 receiver 还是指针 receiver 应保持语义一致。
- `go.official.sync-functions`：API 通常应同步返回结果；异步行为要由调用方控制。
- `go.official.useful-test-failures`：测试失败信息要能定位函数、输入、got/want 和差异。

### Go Test Comments（11）

- `go.test.human-readable-subtests`：子测试名称应让人能看懂场景，而不是机械索引。
- `go.test.compare-full-structures`：优先比较完整结构，避免只断言部分字段漏掉回归。
- `go.test.compare-stable-results`：测试应比较稳定结果，避免依赖 map 顺序、时间等不稳定因素。
- `go.test.equality-diffs`：复杂对象比较失败时应输出有用 diff。
- `go.test.got-before-want`：测试输出和断言应保持 got 在前、want 在后。
- `go.test.identify-function`：失败信息应能看出正在测试哪个函数或行为。
- `go.test.identify-input`：失败信息应包含导致失败的关键输入。
- `go.test.keep-going`：能继续收集多个失败时用 t.Error；无法继续时才 t.Fatal。
- `go.test.print-diffs`：复杂比较失败时打印 diff，而不只是 true/false。
- `go.test.table-driven-vs-multiple`：表驱动和多个测试函数应按可读性选择，不盲目套表。
- `go.test.error-semantics`：测试 error 应验证语义，例如 errors.Is/As，而不只比字符串。

### Google Go Best Practices（37）

- `google.bp.function-method-names`：函数和方法名应表达行为，避免重复包/类型上下文。
- `google.bp.avoid-repetition`：命名和 API 应避免重复已经由上下文表达的信息。
- `google.bp.test-double-packages`：测试替身和 helper 包应简单、局部、避免过度框架化。
- `google.bp.package-size`：包大小应服务内聚性，过大时考虑拆分。
- `google.bp.import-protobuf`：protobuf 消息和 stub 的导入/使用应保持清晰边界。
- `google.bp.error-structure`：错误结构应便于调用方判断和处理。
- `google.bp.add-error-info`：返回错误时应添加有用上下文。
- `google.bp.sentinel-placement`：sentinel error 的定义位置应服务 API 使用者。
- `google.bp.logging-errors`：不要既记录错误又原样返回导致重复日志。
- `google.bp.program-init`：初始化流程应显式、可测试、失败清晰。
- `google.bp.checks-panics`：程序不变量检查和 panic 应限制在不可恢复场景。
- `google.bp.when-to-panic`：只有真正不可恢复或编程错误场景才 panic。
- `google.bp.doc-conventions`：文档应说明行为、参数、并发、安全和错误语义。
- `google.bp.doc-params-config`：参数和配置文档应解释默认值、范围和影响。
- `google.bp.doc-contexts`：涉及 context 的 API 文档应说明取消、超时和值语义。
- `google.bp.doc-concurrency`：并发安全性和调用约束应写清楚。
- `google.bp.doc-cleanup`：需要调用方清理的资源必须在文档中说明。
- `google.bp.doc-errors`：公开 API 应说明可能返回的关键错误语义。
- `google.bp.function-argument-lists`：参数列表过长时考虑配置对象或选项结构。
- `google.bp.option-structure`：多个可选参数可用 options/config 结构表达。
- `google.bp.variadic-options`：可变 options 适合扩展性 API，但不应过度使用。
- `google.bp.cli-complex`：复杂 CLI 应有清晰子命令、flag 和错误体验。
- `google.bp.test-in-test`：断言和失败控制应尽量留在 Test 函数层。
- `google.bp.validation-apis`：测试验证 helper 应可扩展且失败信息清晰。
- `google.bp.real-transports`：集成测试尽量使用真实传输层以减少 mock 偏差。
- `google.bp.t-error-vs-fatal`：t.Error 和 t.Fatal 的选择取决于失败后是否还能继续。
- `google.bp.test-helper-errors`：测试 helper 中处理错误要保留调用位置和上下文。
- `google.bp.scoped-test-setup`：测试 setup 应尽量局部化，避免共享状态污染。
- `google.bp.testmain`：只有确有全局测试生命周期需求时才用 TestMain。
- `google.bp.string-plus`：简单字符串拼接用 + 更直接。
- `google.bp.string-sprintf`：需要格式化多个值时用 fmt.Sprintf。
- `google.bp.constant-strings`：重复使用或有语义的字符串应考虑常量。
- `google.bp.global-state`：全局状态应最小化并保持可测试。
- `google.bp.default-instance`：默认实例 API 应避免隐藏状态和测试困难。
- `google.bp.unnecessary-interfaces`：不要为了测试或习惯过早抽象接口。
- `google.bp.interface-ownership`：接口归属和可见性应服务调用方。
- `google.bp.effective-interfaces`：接口应小、稳定、表达行为。

### Google Go Style Guide（38）

- `google.naming.receiver-names`：receiver 名应短且同类型一致。
- `google.naming.repetition`：避免包名、类型名、变量名重复上下文。
- `google.commentary.comment-line-length`：注释行没有硬性长度限制，但应保持阅读舒适。
- `google.commentary.doc-comments`：文档注释应面向 API 使用者解释用途和行为。
- `google.commentary.package-comments`：package 注释应解释包的整体职责。
- `google.errors.returning-errors`：函数是否返回 error 应基于调用方能否处理失败。
- `google.errors.in-band-errors`：不要用特殊正常返回值来表达错误语义。
- `google.language.zero-value-fields`：结构体字面量中是否省略零值字段应服务可读性。
- `google.language.nil-slices`：空 slice 通常可用 nil slice，但 API 语义可能例外。
- `google.language.copying`：复制带锁或共享状态的值前要理解所有权和副作用。
- `google.language.must-functions`：Must 风格函数只适合初始化或测试等失败即不可恢复场景。
- `google.language.goroutine-lifetimes`：goroutine 应有明确退出条件和生命周期归属。
- `google.language.interfaces`：接口应在真正需要抽象时引入，通常由使用方拥有。
- `google.language.generics`：泛型应降低重复或提升类型安全，不应增加无谓复杂度。
- `google.language.pass-values`：值/指针传递取决于大小、可变性和语义。
- `google.language.receiver-type`：receiver 用值还是指针应保持语义一致。
- `google.language.sync-functions`：API 默认应同步完成；异步行为要由调用方清晰控制。
- `google.language.type-aliases`：type alias 主要用于迁移/兼容，不应滥用。
- `google.language.use-q`：需要清楚展示字符串内容时优先用 %q。
- `google.libs.flags`：命令行 flag 设计应清晰、稳定、可组合。
- `google.libs.logging`：日志策略应一致，避免混乱级别和重复记录错误。
- `google.libs.contexts`：context 只用于请求范围取消、超时和元数据传播。
- `google.test.identify-function`：测试失败信息应说明正在验证哪个函数/行为。
- `google.test.identify-input`：测试失败信息应包含关键输入。
- `google.test.full-structure`：复杂对象优先比较完整结构，避免遗漏字段。
- `google.test.stable-results`：测试只比较稳定结果，避免顺序/时间等不稳定因素。
- `google.test.keep-going`：能继续收集失败时不要过早 Fatal。
- `google.test.equality-diffs`：相等比较失败时应提供可诊断 diff。
- `google.test.level-of-detail`：测试失败信息要有足够细节但不过度噪音。
- `google.test.print-diffs`：复杂差异应打印 diff。
- `google.test.error-semantics`：测试错误时应验证语义而非脆弱字符串。
- `google.test.subtests`：子测试应让测试场景结构更清楚。
- `google.test.subtest-names`：子测试名称应可读并描述场景。
- `google.test.table-driven`：表驱动测试应提升清晰度，而不是制造复杂表格。
- `google.test.data-driven-cases`：数据驱动用例应把输入、期望和名称表达清楚。
- `google.test.identifying-row`：表驱动失败应能定位具体 case。
- `google.test.package-same`：是否同包测试取决于是否需要访问内部实现。
- `google.test.package-external`：外部包测试适合验证公开 API。

### Uber Go Style Guide（23）

- `uber.guideline.interface-compliance`：关键类型可用编译期断言证明实现了接口。
- `uber.guideline.receivers-interfaces`：receiver 选择会影响方法集和接口实现，应一致。
- `uber.guideline.copy-boundaries`：slice/map 穿越 API 边界时要考虑拷贝，避免外部修改内部状态。
- `uber.guideline.use-time`：时间点/时长应使用 time.Time/time.Duration，而不是裸 int/string。
- `uber.guideline.error-types`：错误类型设计应服务调用方匹配和处理。
- `uber.guideline.handle-errors-once`：error 应只处理一次，避免 log 后再返回造成重复处理。
- `uber.guideline.field-tags`：会序列化的 struct 字段应有明确 tag。
- `uber.guideline.no-fire-forget`：不要启动无人等待、无退出条件的 goroutine。
- `uber.style.long-lines`：避免影响阅读的超长行，但不设机械列宽。
- `uber.style.consistency`：同一项目内相似代码应保持一致。
- `uber.style.function-names`：函数名应表达动作和结果。
- `uber.style.import-aliasing`：import alias 只在必要时使用。
- `uber.style.function-ordering`：文件内函数顺序应帮助阅读。
- `uber.style.top-level-vars`：顶层变量声明应清晰分组。
- `uber.style.unexported-global-prefix`：可用前缀标识未导出的全局变量以提示风险。
- `uber.style.nil-valid-slice`：nil slice 是有效空 slice，通常不必构造空 literal。
- `uber.style.reduce-scope`：变量作用域应尽量小。
- `uber.style.naked-parameters`：避免多个裸 bool/int 参数让调用点难读。
- `uber.style.raw-strings`：复杂字符串优先用 raw string 降低转义噪音。
- `uber.style.omit-zero-fields`：struct literal 中可省略无意义零值字段。
- `uber.pattern.test-tables`：表驱动测试应简单、命名清晰、错误信息可诊断。
- `uber.pattern.table-complexity`：不要把简单测试做成复杂表驱动框架。
- `uber.pattern.functional-options`：可选参数很多时可考虑 functional options，但不是默认选择。

