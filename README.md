# go-review

`go-review` 是一个面向 Go 项目的代码审阅编排工具。它把格式化、静态检查、测试、语义规则和可选 LLM 审阅放到同一条可配置流水线里，并输出稳定的 Markdown / JSON 报告。

## 默认行为

裸命令默认执行安全修复：

```bash
go-review
```

等价于：

```bash
go-review fix --profile fast
```

如果只想检查、不允许修改源码，显式运行：

```bash
go-review check --profile fast
```

`fix` 只会执行同时满足 `allow_fix: true` 和 `fix_safety: safe` 的步骤。默认安全修复主要是格式化和 import 整理；lint、test、semantic、LLM review 默认只报告或按各自策略执行，不会绕过验证。

## 快速开始

安装：

```bash
go build -o ~/bin/go-review ./cmd/go-review
```

`go.lint` 默认依赖 `golangci-lint`：

```bash
go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@latest
```

在 Go 项目根目录初始化：

```bash
go-review init
```

生成结构：

```text
.go-review/
  go-review.yaml
  rules/
    catalog.json
    llm-default.json
    llm-custom.json
    semantic-default.yaml
    semantic-custom.yaml
```

运行：

```bash
go-review
```

默认 `fast` profile 执行：

```text
format-check -> test
```

## 常用命令

```bash
go-review                         # 默认 safe fix：fix --profile fast
go-review check --profile fast    # 只读快速检查
go-review check --profile ci      # 只读 CI 检查
go-review fix --profile ci        # safe fix 后跑 CI profile
go-review --profile review        # safe fix，并运行启用的 LLM review steps
go-review update                  # 检测并确认升级本地二进制
go-review rules validate          # 校验规则 catalog
```

指定配置、工作目录或报告目录：

```bash
go-review check \
  --config .go-review/go-review.yaml \
  --profile ci \
  --workdir . \
  --report-dir /tmp/go-review-report
```

## 最小目录

默认本地结构保持精简：

```text
.go-review/
  go-review.yaml
  rules/
    catalog.json
    llm-default.json
    llm-custom.json
    semantic-default.yaml
    semantic-custom.yaml
  reports/
    latest.md
    latest.json
```

`reports/latest.md` 给人看，回答是否通过、失败项、已做的 safe fix、LLM/Claude 摘要和下一步。`reports/latest.json` 给 CI 或其它工具消费。

默认不生成 `artifacts/`、`reports/runs/`、`latest.llm.md` 或 `latest.process.md`。完整 stdout/stderr/prompt 只应在显式 debug 或 keep-artifacts 模式中保留。

## 配置分工

`.go-review/go-review.yaml` 定义怎么跑：

- adapters：接入 `go.lint`、`go.test`、`go.semantic`、`llm.review`、`llm.claude` 等。
- steps：声明执行节点、`on_fail`、`allow_fix`。
- profiles：组合不同场景，例如 `fast`、`ci`、`nightly`、`review`。
- defaults：声明 `workdir` 和 timeout。

`.go-review/rules/` 定义查什么：

- `catalog.json`：本地轻量规则索引，报告中的 `rule_id` 可回查这里。
- `llm-default.json`：框架维护的 C 类 LLM review 规则。
- `llm-custom.json`：团队自定义 LLM review 规则。
- `semantic-default.yaml`：框架维护的内置 semantic 规则。
- `semantic-custom.yaml`：团队自定义的参数化 semantic 规则。

`golangci.yml` 默认不生成。默认 lint 规则通过 `go-review.yaml` 的 adapter args 明确声明；项目确实需要额外排除或配置时，可以自行维护独立 golangci 配置并在 adapter args 里引用。

## LLM Review

LLM steps 默认存在但禁用：

- `llm.review`：默认调用 `codex exec`。
- `llm.claude`：默认调用 `claude -p` 做第二模型复盘。

启用前需要本机 CLI 已安装并登录，然后把对应 step 改为 `enabled: true`。LLM 会读取 `latest.md`、`.go-review/rules/llm-default.json` 和 `.go-review/rules/llm-custom.json`。

## 规则管理

仓库源规则位于：

```text
rules/go-rules.json
```

常用命令：

```bash
go-review rules validate --catalog rules/go-rules.json
go-review rules list --catalog rules/go-rules.json
go-review rules get <rule-id> --catalog rules/go-rules.json
go-review rules render-doc --catalog rules/go-rules.json --out docs/quality/go-rule-catalog.md
```

## 验证

本仓库测试：

```bash
go test ./...
```

如果本机 Go build cache 不可写，可以指定临时 cache：

```bash
GOCACHE=/private/tmp/go-review-gocache go test ./...
```

## 文档

- [产品说明](docs/product/go-code-quality-governance.md)
- [为什么需要 go-review](docs/product/why-go-review.md)
- [后端设计](docs/backend/rule-engine-and-autofix.md)
- [质量基线](docs/quality/go-code-quality-baseline.md)
- [消费方接入](docs/adoption/consumer-project.md)
