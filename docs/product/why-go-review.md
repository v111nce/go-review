# 为什么需要 go-review

`go-review` 的产品定位是 Go code-review 编排层：它不替代成熟 linter，也不把某个单一工具包装成平台本体，而是把 lint、format、test、安全扫描、团队语义规约和报告输出放进同一条可配置 pipeline。

## 用户问题：质量工具是孤岛

Go 生态已经有成熟工具：

- `golangci-lint` 聚合通用 lint 和 formatter。
- `go test` 负责测试、coverage 和 race。
- `govulncheck` 负责漏洞扫描。
- `gosec` 负责常见安全问题检查。
- 团队还可能有内部脚本、自研 analyzer 或历史工具。

这些工具各自有价值，但用户在真实项目里遇到的问题是：

| 问题 | 用户影响 |
| --- | --- |
| 配置分散 | 本地、PR、nightly 很容易跑的不是同一组规则 |
| 输出分散 | 失败信息散落在不同工具日志里，难以给开发者和 LLM 提供稳定修复上下文 |
| 顺序不清 | 无法统一表达先 format / lint，再 test / security，或失败后继续收集其它问题 |
| 修复策略不统一 | 有些工具能安全修复，有些只能报告，缺少统一的 safe / review / none 语义 |
| 自定义规约难沉淀 | 团队特定规则既可能是 Go analyzer，也可能是外部命令，需要同一条门禁承载 |

因此，go-review 解决的是“多工具质量门禁如何统一编排和报告”的问题，而不是“重新实现所有 Go lint 规则”。

## 不是直接替代 golangci-lint

`golangci-lint` 是 Go 项目的成熟 lint / formatter runner，应当优先复用。它适合承担：

- `go vet`、`staticcheck`、`revive`、`errcheck` 等通用 lint。
- `gofmt`、`goimports`、`gofumpt`、`gci` 等 formatter。
- `golangci-lint run --fix` 可证明安全的 formatter / linter 修复。

但它不是完整 review pipeline：

| 边界 | 为什么仍需要 go-review |
| --- | --- |
| 不替代 `go test` | 测试、coverage、race 应该作为独立 step，拥有自己的 artifact 和失败策略 |
| 不负责跨工具 profile | fast / ci / nightly 需要组合 lint、test、security、semantic 和报告输出 |
| 不负责统一 LLM 修复上下文 | go-review 需要把所有工具结果归一成 Markdown / JSON / LLM report |
| 不作为团队 semantic 规则主扩展面 | 复杂团队规则长期优先写成 `go/analysis` analyzer；当前不把 `.go-review/semantic/custom.yaml` 伪装成通用扩展面 |

所以目标关系是：

```text
golangci-lint = 通用 lint/format runner
go/analysis   = 自定义 Go semantic analyzer 底座
go-review     = 多工具编排、报告、门禁和安全修复事务
```

## 为什么需要 go/analysis

团队语义规约通常不是文本匹配能可靠解决的问题。例如：

- 禁止直接调用某个 SDK 或内部包函数。
- 某些 package 不能 import 某些层。
- 函数形状、context 参数、错误处理、对象访问需要理解 AST 和类型。
- 某些修复只有在类型和读写位置明确时才安全。

这类规则应该用 Go 官方生态的 `go/analysis` 编写 analyzer。`go/analysis` 负责 AST、type info、diagnostic 和 `SuggestedFix`；go-review 负责把 analyzer 结果映射成统一 review result，并放进 pipeline。

当前实现中的 `.go-review/semantic/custom.yaml` 只支持有限的配置式规则 kind，例如 `no-direct-call`。它不是完整的任意 YAML 规则语言；adapter 配置里的 `parser` 也只是兼容的内置规则选择入口，不是插件机制。当前 semantic step 遵循单结果契约，只报告首个 failing finding，review-only semantic 规则也不自动改写代码。更复杂规则、多 diagnostics 输出或 semantic autofix 需要后续单独设计 analyzer/runtime 和 safe fix transaction。

## go-review 的产品边界

| 层级 | 负责什么 | 不负责什么 |
| --- | --- | --- |
| `golangci-lint` | 通用 lint、formatter、部分 safe fix | 不跑测试，不生成 go-review 统一报告，不管理跨工具 pipeline |
| `go/analysis` | 长期用于编写 Go 语义 analyzer，产出 diagnostic / SuggestedFix | 不负责 profile、artifact、报告归一或 CI 门禁 |
| `go-review` | adapter、steps、profiles、`on_fail`、artifact、safe fix transaction、中文/LLM 报告 | 不重复造通用 lint 规则，不把所有 semantic 规则硬编码进 runner |

## 推荐运行模型

一份项目配置定义所有工具入口和执行场景：

```yaml
adapters:
  - id: go.lint
    type: cmd
    command: golangci-lint
    args: [run]
    capabilities: [check]

  - id: go.test
    type: cmd
    command: go
    args: [test, ./...]
    capabilities: [test]

  - id: go.semantic
    type: go.semantic
    capabilities: [check]

steps:
  - id: lint
    adapter: go.lint
    on_fail: continue

  - id: test
    adapter: go.test
    on_fail: continue

  - id: semantic
    adapter: go.semantic
    on_fail: continue

profiles:
  - name: fast
    steps: [lint, test]

  - name: ci
    steps: [lint, test, semantic]
```

安全自动修复仍由 go-review 管控：只有 step 显式允许 `allow_fix: true`，且 adapter / 规则声明 `fix_safety: safe` 时，`go-review fix` 才能应用修改。format / import 类修复可以由内置 `go.format` 或 `golangci-lint run --fix` 承担；review-only semantic 规则默认只报告。

## 用户获得的价值

| 价值 | 说明 |
| --- | --- |
| 一条命令 | 本地、CI、nightly 都通过 `go-review check/fix --profile ...` 运行 |
| 一份报告 | lint、test、security、semantic 的结果进入统一 Markdown / JSON / LLM context |
| 独立失败收集 | `on_fail: continue` 让 format、test、semantic、安全扫描互不遮挡 |
| 成熟工具复用 | 通用 lint/format 不重写，优先复用 `golangci-lint` |
| 自定义规则可演进 | 团队规则当前可用已支持的 semantic kind 或 `cmd` 外部工具接入；复杂 analyzer 走后续 `go/analysis` runtime |
| 安全修复边界 | safe fix 可以自动应用，review / none 只报告，失败可回滚 |

## 关联产品模块

本文是产品定位说明，不直接产生新的 implementation story。它解释现有能力模块为什么同时存在，并把后续实现追踪回 [Go 代码质量规约治理](go-code-quality-governance.md) 中的 Story 候选。

| Module Key | 关系 |
| --- | --- |
| `tool-adapter-platform` | 解释为什么所有工具都通过 adapter 接入，而不是绑定单一 linter |
| `review-pipeline` | 解释为什么需要 profile、step、失败策略和跨工具编排 |
| `custom-rule-adapters` | 解释为什么团队 semantic 规则需要已实现 semantic kind、后续 `go/analysis` runtime 或外部工具承载 |
| `policy-and-autofix` | 解释为什么 safe fix、review-only 和 rollback 要统一治理 |
| `regression-gates` | 解释为什么本地、CI、nightly 需要复用同一套门禁配置 |

## 与核心能力的关系

本文解释“为什么需要 go-review”。具体产品能力、模块和 Story 候选见 [Go 代码质量规约治理](go-code-quality-governance.md)。后端运行对象、adapter 契约和 `go/analysis` / `golangci-lint` 分工见 [Review Pipeline 与自动修复](../backend/rule-engine-and-autofix.md)。质量门禁和验证基线见 [Go 代码质量基线](../quality/go-code-quality-baseline.md)。
