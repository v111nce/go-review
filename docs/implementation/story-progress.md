# Story Progress

Status legend:

- `ready`: story is authored and implementation has not started.
- `in-progress`: current team run has implementation work underway.
- `verified`: executable evidence has passed for the story.
- `blocked`: implementation is waiting on an explicit dependency or decision.

| Story | Product Source | Current slice | Status | Evidence |
| --- | --- | --- | --- | --- |
| STORY-001 `tool-adapter-platform-core` | [工具接入平台](../product/go-code-quality-governance.md#工具接入平台) | Milestones 0-1 / Lane A+B | in-progress | Pending code-lane `go test ./...`; fixture contract in `testdata/fixtures/regression-gates/configs/go-review.yaml` |
| STORY-002 `tool-adapter-platform-output-normalization` | [工具接入平台](../product/go-code-quality-governance.md#工具接入平台) | Milestone 2 / Lane B+C | in-progress | Golden report targets in `testdata/fixtures/regression-gates/expected-reports/` |
| STORY-003 `review-pipeline-dag-execution` | [Review Pipeline 编排](../product/go-code-quality-governance.md#review-pipeline-编排) | Milestone 3 / Lane C | in-progress | Fixture profiles include dependent and independent steps |
| STORY-004 `review-pipeline-profiles` | [Review Pipeline 编排](../product/go-code-quality-governance.md#review-pipeline-编排) | Milestone 3 / Lane C+D | in-progress | `local`, `ci`, `main`, and `nightly` profiles defined in fixture config |
| STORY-005 `custom-rule-adapters-sdk` | [自定义规约 Adapter](../product/go-code-quality-governance.md#自定义规约-adapter) | Later milestone | ready | Not in Milestones 0-3 |
| STORY-006 `custom-rule-adapters-command` | [自定义规约 Adapter](../product/go-code-quality-governance.md#自定义规约-adapter) | Milestone 1 partial / Lane B+D | in-progress | `fixture.security-*` command adapters use `testdata/fixtures/regression-gates/scripts/fake-tool.sh` |
| STORY-007 `policy-and-autofix-safety-levels` | [策略与安全自动修复](../product/go-code-quality-governance.md#策略与安全自动修复) | Later milestone, minimal metadata now | ready | `go.format` fixture marks `fix_safety: safe`; full policy precedence deferred |
| STORY-008 `policy-and-autofix-transaction` | [策略与安全自动修复](../product/go-code-quality-governance.md#策略与安全自动修复) | Later milestone | ready | Transaction/golden autofix tests deferred |
| STORY-009 `regression-gates-local-fast-check` | [回归门禁](../product/go-code-quality-governance.md#回归门禁) | Milestone 3 / Lane D | verified | `go run ./cmd/go-review check --config testdata/fixtures/regression-gates/configs/go-review.yaml --profile local --workdir <abs>/compliant-project` passed |
| STORY-010 `regression-gates-pr-quality-check` | [回归门禁](../product/go-code-quality-governance.md#回归门禁) | Milestone 3 / Lane D | verified | `go run ./cmd/go-review check --config testdata/fixtures/regression-gates/configs/go-review.yaml --profile ci --workdir <abs>/violating-project` failed as expected on format gate |
| STORY-011 `regression-gates-scheduled-full-regression` | [回归门禁](../product/go-code-quality-governance.md#回归门禁) | Milestone 3 / Lane D | in-progress | Nightly profile and `nightly-pass.golden.json` fixture; full nightly CLI smoke pending scheduler/profile completion beyond current stop-on-failure behavior |

## Current verification snapshot

Lane D owns fixtures/docs/final evidence. At this point the repository began documentation-only and other lanes are still adding Go code. Lane D verification therefore separates fixture/doc validation from code-lane smoke checks:

| Check | Status | Evidence |
| --- | --- | --- |
| Fixture projects compile/test independently | verified | Compliant fixture `go test ./...` passed; violating fixture `go test ./...` failed with intentional failure. |
| Whole repo Go tests | verified | `go test ./...` passed for cmd, integration, adapter, config, engine, fix, pipeline, report, and result packages. |
| CLI help smoke | verified | `go run ./cmd/go-review --help` printed usage. |
| CLI profile smoke | partially verified | Local compliant profile passed; CI violating profile failed as expected on `go.format`; nightly full-profile smoke remains pending. |
| Docs traceability | verified | `docs/quickstart.md`, `docs/adr/0001-contract-first-review-pipeline.md`, this file, quality baseline updates, and fixture README map back to product/backend/quality docs. |
