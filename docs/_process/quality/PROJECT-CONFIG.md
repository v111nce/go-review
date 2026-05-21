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
