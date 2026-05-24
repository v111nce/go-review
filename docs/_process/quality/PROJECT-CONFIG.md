# Quality Project Config

## 当前质量对象

Go code-review 编排平台。

## 检查分层

| 层级 | 目标 | 状态 |
| --- | --- | --- |
| 本地快速检查 | 开发者提交前反馈 | planned |
| PR 门禁 | 合入前阻断违规 | planned |
| 主分支门禁 | 防止主干退化 | planned |
| 定时全量回归 | 捕获慢速、全量和安全问题 | planned |

## 安全扫描策略

| 场景 | 策略 |
| --- | --- |
| PR | 跑轻量安全检查，避免 PR 反馈过慢 |
| Nightly | 跑完整 `gosec`、`govulncheck` 和长耗时安全扫描 |
| 高风险项目 | 可以把完整安全扫描提升为 PR 必跑 |

## 过程记录

当前尚未执行真实测试或 CI。后续运行记录应写入 `docs/_process/quality/E2E-RUNS.md` 或项目约定的质量运行记录文件。

## 2026-05-19 Implementation fixture baseline

本次 Milestones 0-3 实现使用 `testdata/fixtures/regression-gates/` 作为第一批 CLI / adapter / pipeline 质量证据：

- `configs/go-review.yaml` 复用同一配置定义 `fast`、`ci`、`main`、`nightly` profile，避免本地和 CI 漂移。
- `compliant-project/` 是正例，目标用于 fast/nightly pass smoke。
- `violating-project/` 是反例，目标用于 CI gate fail smoke。
- `scripts/fake-tool.sh` 是可预测外部命令 fixture，用于 `cmd` adapter 成功、失败和超时测试。
- `expected-reports/` 保存终端/JSON report golden。

待代码 lanes 暴露 CLI 后，质量运行记录应保存 `go test ./...`、`go run ./cmd/go-review --help` 和 fixture profile smoke 的输出。

## 2026-05-22 Go rule catalog seed

新增 `docs/quality/go-rule-catalog.md` 作为 Go 官方、Google Go Style 和 Uber Go Style 的规则 catalog 初版。该 catalog 只登记来源、规则说明、处理方式、推荐承接和默认策略；未声明全部规则已经实现。首批落地策略是：优先复用 `golangci-lint` 已覆盖规则，保留 `go test` 独立 step，只把工具覆盖不了且能稳定判断的规则推进到 `go.semantic`。

## 2026-05-22 Full catalog index update

用户确认需要全量数据，并指出大量优秀 Go 规范只能由大模型/人工判断，不能都写成代码规则。因此 `docs/quality/go-rule-catalog.md` 从 seed catalog 扩展为章节级全量索引：登记 Go Code Review Comments、Go Test Comments、Google Go Style Decisions、Google Go Best Practices、Uber Go Style Guide 的规则说明，并用保守处理方式区分 `tool-golangci`、`tool-golangci-config`、`tool-go-test`、`tool-external`、`tool-semantic`、`llm-review`、`candidate`。核心决策：go-review 不伪装所有规则都可机器检测；只有确定性、可定位、无需人工解释的规则进入工具门禁，设计型规则进入规范文档和 LLM/人工报告。


## 2026-05-22 Rule catalog JSON CRUD

用户确认规则 catalog 应做成 JSON 源数据并提供 CRUD。已新增 `rules/go-rules.json` 作为结构化规则源，新增 `go-review rules list/get/add/upsert/delete/validate/render-doc` 命令作为文件级 CRUD 和 Markdown 渲染入口。后续定时更新机制应只生成候选变更报告，不自动把新上游规则启用为 gate；检测报告继续要求携带 catalog `rule_id`。


## 2026-05-22 First implemented catalog rule

规则 catalog 新增 `implemented` 布尔字段，要求“已实现”必须由代码和测试支撑。首条标记为已实现的规则是 `team.semantic.max-params`：`go.semantic` 的 `max-params` analyzer 已支持入参阈值检测，配置中使用 catalog id 时，检测报告输出同一个 `rule_id`，便于后续聚合、豁免和文档回链。


## 2026-05-23 Implement deterministic catalog rules

按“能实现的全实现”要求，当前 JSON catalog 中确定性工具规则已全部实现并标记 `implemented: true`：`go.official.gofmt`、`go.official.imports`、`go.official.handle-errors`、`team.semantic.max-params`。`go.lint` 现在会把 gofmt/goimports/gci/errcheck 对应输出映射到 catalog rule id；`go.semantic` 的 max-params 支持使用 catalog id 并在报告里保持一致。`llm-review` 和 `candidate` 规则不标实现，避免把设计判断或未评估 linter 误判率的规则伪装为硬 gate。
