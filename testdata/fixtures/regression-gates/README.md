# Regression Gate Fixtures

These fixtures are owned by Lane D and provide docs-traceable evidence for the first Go code-review delivery slice.
They are intentionally small so adapter, pipeline, report, and CLI lanes can reuse them without coupling tests to one concrete tool implementation.

Traceability:

- Product module: `regression-gates`
- Stories: STORY-009, STORY-010, STORY-011
- Backend contract: `docs/backend/rule-engine-and-autofix.md` pipeline, adapter, result, and artifact contracts
- Quality baseline: `docs/quality/go-code-quality-baseline.md` gate layering

## Fixture map

| Path | Purpose | Expected gate |
| --- | --- | --- |
| `compliant-project/` | Minimal Go module that should pass fast/ci/nightly smoke profiles. | pass |
| `violating-project/` | Minimal Go module with deterministic formatting and test failures. | fail |
| `semantic-violating-project/` | Minimal Go module with direct `os.Getenv` usage for the `go.semantic` example rule. | fail |
| `autofix-project/` | Minimal Go module with safe formatting drift and passing tests for `fix --profile fast`. | pass after fix |
| `configs/go-review.yaml` | Contract fixture for fast, ci, main, and nightly profile selection. | profile-dependent |
| `scripts/fake-tool.sh` | Deterministic fake external tool for `cmd` adapter tests. | pass/fail by args |
| `expected-reports/*.golden.*` | Stable report artifacts for early terminal/JSON snapshot tests. | deterministic |

## Verification intent

Once code lanes expose `go-review`, these fixtures should support:

```bash
go test ./...
go run ./cmd/go-review --help
go run ./cmd/go-review check --config testdata/fixtures/regression-gates/configs/go-review.yaml --profile fast --workdir testdata/fixtures/regression-gates/compliant-project
go run ./cmd/go-review check --config testdata/fixtures/regression-gates/configs/go-review.yaml --profile ci --workdir testdata/fixtures/regression-gates/violating-project
go run ./cmd/go-review check --config testdata/fixtures/regression-gates/configs/go-review.yaml --profile nightly --workdir testdata/fixtures/regression-gates/compliant-project
go run ./cmd/go-review check --config testdata/fixtures/regression-gates/configs/go-review.yaml --profile semantic --workdir testdata/fixtures/regression-gates/semantic-violating-project
go run ./cmd/go-review fix --config testdata/fixtures/regression-gates/configs/go-review.yaml --profile fast --workdir testdata/fixtures/regression-gates/autofix-project
```

The fixture uses `cmd` steps and `go.lint` by adapter ID to preserve the platform boundary: concrete tools stay behind adapters and the pipeline core remains tool-agnostic.
The `semantic` profile proves a custom semantic rule can affect the same gate contract; the `autofix-project` proves safe fixes are validated after application.
