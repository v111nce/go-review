# Open Questions

| ID | Domain | Question | Impact | Status |
| --- | --- | --- | --- | --- |
| PO-1 | Product | 第一版常用 adapter 是否按 `cmd`、`go.lint`、`go.arch`、`go.security`、`go.test`、`go.semantic`、`report.github` 交付？ | 已决定：第一版按这组常用 adapter 交付；`go.lint`/format 优先复用 `golangci-lint`，`go.semantic` 基于 `go/analysis` analyzer，`go.test` 独立保留 | closed |
| PO-2 | Product | 第一版是否提供 adapter 市场/安装能力，还是只支持项目内本地 adapter？ | 已决定：第一版只支持项目内本地 adapter，不做市场 | closed |
| BO-1 | Backend | adapter 和 pipeline 的配置 schema 如何稳定？ | 已决定：第一版采用简单稳定 YAML schema | closed |
| QO-1 | Quality | PR 阶段是否必须运行 `govulncheck` 和 `gosec`，还是放到定时全量回归？ | 已决定：PR 轻量安全检查，nightly 全量安全扫描 | closed |
