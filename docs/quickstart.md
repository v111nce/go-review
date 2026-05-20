# Quickstart: Go Review Pipeline

This project implements a tool-agnostic Go code-review orchestration platform. The first delivery slice focuses on a runnable local/CI/nightly review pipeline with fixtures and stable evidence.

## Current implementation slice

The team is implementing Milestones 0-3 from `.omx/plans/ralplan-docs-implementation-20260519T091728Z.md`:

1. Go module and thin `go-review` CLI bootstrap.
2. Config, adapter registry, command adapter, and minimal `go.format` wrapper.
3. Normalized result/report artifacts.
4. Pipeline DAG/profile execution for `local`, `ci`, `main`, and `nightly` gates.

## Fixture-backed commands

After the code lanes expose `cmd/go-review`, use the Lane D fixtures for smoke checks:

```bash
go test ./...
go run ./cmd/go-review --help
go run ./cmd/go-review check \
  --config testdata/fixtures/regression-gates/configs/go-review.yaml \
  --profile local \
  --workdir testdata/fixtures/regression-gates/compliant-project
go run ./cmd/go-review check \
  --config testdata/fixtures/regression-gates/configs/go-review.yaml \
  --profile ci \
  --workdir testdata/fixtures/regression-gates/violating-project
```

Expected behavior:

- `local` on `compliant-project` passes.
- `ci` on `violating-project` fails and emits step, adapter, rule/location when available, reason, and artifact path.
- `nightly` can add long-running steps without changing `local` or `ci` behavior.

## Traceability

- Product: `docs/product/go-code-quality-governance.md#回归门禁`
- Backend: `docs/backend/rule-engine-and-autofix.md`
- Quality: `docs/quality/go-code-quality-baseline.md#门禁分层`
- Stories: STORY-009, STORY-010, STORY-011
- Fixture contract: `testdata/fixtures/regression-gates/README.md`
