# Story Progress

Status legend:

- `ready`: story is authored and implementation has not started.
- `in-progress`: implementation exists but one or more acceptance checks are still missing.
- `verified`: executable evidence has passed for the story.
- `blocked`: implementation is waiting on an explicit dependency or decision.

| Story | Product Source | Current slice | Status | Evidence |
| --- | --- | --- | --- | --- |
| STORY-001 `tool-adapter-platform-core` | [工具接入平台](../product/go-code-quality-governance.md#工具接入平台) | CLI/config/adapter core | verified | `go test ./...`; `internal/config`, `internal/engine`, `internal/adapter`; fixture config runs `cmd`, `go.format`, and `go.semantic` adapters. |
| STORY-002 `tool-adapter-platform-output-normalization` | [工具接入平台](../product/go-code-quality-governance.md#工具接入平台) | Normalized result/report fields | verified | CLI output and `internal/report` include adapter, step, rule, location, message, fix metadata, gate, artifact paths, human Markdown, LLM repair Markdown, and JSON reports; report tests pass in `go test ./...`. |
| STORY-003 `review-pipeline-dag-execution` | [Review Pipeline 编排](../product/go-code-quality-governance.md#review-pipeline-编排) | DAG dependency ordering/failure policy | verified | `internal/pipeline` tests cover ordering, ready batches, cycles, dependency validation, failure policies; engine executes profile steps in dependency order. |
| STORY-004 `review-pipeline-profiles` | [Review Pipeline 编排](../product/go-code-quality-governance.md#review-pipeline-编排) | `fast`, `ci`, `main`, `nightly`, `semantic` profiles | verified | Fixture config profile tests; smoke commands for fast/ci/nightly/semantic profiles passed. |
| STORY-005 `custom-rule-adapters-sdk` | [自定义规约 Adapter](../product/go-code-quality-governance.md#自定义规约-adapter) | Minimal semantic adapter proof; target architecture is `go/analysis` analyzer runtime | verified | Current `go.semantic` detects direct `os.Getenv`; `semantic-violating-project` fails with rule `semantic.no-direct-os-getenv`, file/line/column, reason, suggestion, and `review` safety. |
| STORY-006 `custom-rule-adapters-command` | [自定义规约 Adapter](../product/go-code-quality-governance.md#自定义规约-adapter) | External command adapter | verified | `cmd` adapter tests cover stdout/stderr, env/workdir, exit code, timeout, artifacts; fixture `fixture.security-*` adapters run `scripts/fake-tool.sh`. |
| STORY-007 `policy-and-autofix-safety-levels` | [策略与安全自动修复](../product/go-code-quality-governance.md#策略与安全自动修复) | Canonical safety + CLI display | verified | `ParseFixSafety` aliases and `internal/fix.Policy` precedence tests pass; CLI prints `fix_available` and `fix_safety`; `fix` applies only allowed `safe` `go.format` step. |
| STORY-008 `policy-and-autofix-transaction` | [策略与安全自动修复](../product/go-code-quality-governance.md#策略与安全自动修复) | Safe format transaction + validation rollback | verified | `internal/fix` covers overlap/rollback; engine rollback test restores formatted file when dependent `go test` fails; autofix fixture passes check→fix→test. |
| STORY-009 `regression-gates-local-fast-check` | [回归门禁](../product/go-code-quality-governance.md#回归门禁) | Fast check/fix profile | verified | `go run ./cmd/go-review check --profile fast --workdir <abs>/compliant-project` passed; `fix --profile fast` on `autofix-project` applies safe gofmt and validates tests. |
| STORY-010 `regression-gates-pr-quality-check` | [回归门禁](../product/go-code-quality-governance.md#回归门禁) | CI/main profile | verified | `go run ./cmd/go-review check --profile ci --workdir <abs>/violating-project` exited non-zero on `go.format` and emitted artifact paths. |
| STORY-011 `regression-gates-scheduled-full-regression` | [回归门禁](../product/go-code-quality-governance.md#回归门禁) | Nightly full/long-running profile | verified | `go run ./cmd/go-review check --profile nightly --workdir <abs>/compliant-project` passed full security, custom long rule, and semantic nightly-only steps without changing fast/ci behavior. |

## Current verification snapshot

| Check | Status | Evidence |
| --- | --- | --- |
| First git baseline | verified | Root commit `2a0affd` created after `git init`; `.omx/`, `.idea/`, and generated artifacts remain ignored. |
| Whole repo Go tests | verified | `go test ./...` passed for cmd, integration, adapter, config, engine, fix, pipeline, report, and result packages. |
| CLI help smoke | verified | `go run ./cmd/go-review --help` printed check/fix usage and flags. |
| Fast profile smoke | verified | `check --profile fast` on `compliant-project` passed with format and test steps. |
| CI profile smoke | verified | `check --profile ci` on `violating-project` failed as expected on format gate with non-zero exit. |
| Nightly profile smoke | verified | `check --profile nightly` on `compliant-project` passed format, test, security-lite, full-security, custom-long-rules, and semantic steps. |
| Semantic adapter smoke | verified | `check --profile semantic` on `semantic-violating-project` failed with `semantic.no-direct-os-getenv` and source location. |
| Safe autofix smoke | verified | `fix --profile fast` on `autofix-project` applied gofmt, reran tests, and passed. |
| Transaction rollback | verified | Engine unit test rolls back formatting when dependent validation fails; `internal/fix` unit tests cover overlap and validator rollback. |
| Docs traceability | verified | `docs/quickstart.md`, `docs/backend/rule-engine-and-autofix.md`, fixture README, and this file map implementation evidence back to product/backend/quality docs. |

## Remaining known gaps

- Framework self-check CI is checked in at `.github/workflows/self-check.yml` and dogfoods `go test ./...`, CLI help/version, and the fast/ci/nightly/semantic/fix fixture matrix.
- A GitHub Actions consumer adoption template is checked in at `examples/consumer-go-project/.github/workflows/go-review.yml`; its install command uses the chosen public module path and keeps `v0.1.0` as a placeholder until the first release tag exists.
- Release packaging is checked in at `.github/workflows/release.yml`; it builds stamped cross-platform archives and checksums for tags, but no real `v0.1.0` tag has been cut yet.
- The current semantic adapter is an in-repo AST/type-info example bridge, not a dynamic third-party plugin loader; long-term custom semantic rules should be implemented as `go/analysis` analyzers and run through `go.semantic`.
- SARIF/GitHub reporter depth remains a later enhancement beyond the verified portable terminal/JSON/Markdown report writers.
