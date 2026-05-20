# Quickstart: Go Review Pipeline

This project implements a tool-agnostic Go code-review orchestration platform. The first delivery slice focuses on a runnable local/CI/nightly review pipeline with fixtures and stable evidence.

## Current implementation slice

The current executable slice implements the full first-story set from `.omx/plans/ralplan-docs-implementation-20260519T091728Z.md`:

1. Go module and thin `go-review` CLI bootstrap.
2. Config, adapter registry, command adapter, and minimal `go.format` wrapper.
3. Normalized result/report artifacts.
4. Pipeline DAG/profile execution for `local`, `ci`, `main`, and `nightly` gates.
5. Example `go.semantic` custom rule adapter.
6. Safe `go.format` autofix with validation rollback evidence.

## Fixture-backed commands

Use the regression fixtures for smoke checks:

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
go run ./cmd/go-review check \
  --config testdata/fixtures/regression-gates/configs/go-review.yaml \
  --profile nightly \
  --workdir testdata/fixtures/regression-gates/compliant-project
go run ./cmd/go-review check \
  --config testdata/fixtures/regression-gates/configs/go-review.yaml \
  --profile semantic \
  --workdir testdata/fixtures/regression-gates/semantic-violating-project
go run ./cmd/go-review fix \
  --config testdata/fixtures/regression-gates/configs/go-review.yaml \
  --profile local \
  --workdir testdata/fixtures/regression-gates/autofix-project
```

Expected behavior:

- `local` on `compliant-project` passes.
- `ci` on `violating-project` fails and emits step, adapter, rule/location when available, reason, and artifact path.
- `nightly` can add long-running steps without changing `local` or `ci` behavior.
- `semantic` on `semantic-violating-project` fails with `semantic.no-direct-os-getenv`.
- `fix --profile local` applies only `safe` fixes, reruns validation, and rolls back if a later validation step fails.

## Framework self-check CI

This repository dogfoods `go-review` through `.github/workflows/self-check.yml`.
That workflow is a framework regression guard, not the full consumer-project adoption story:

- It runs `go test ./...` to protect the framework implementation.
- It runs CLI help to protect the user-facing command surface.
- It runs the same fixture-backed local, ci, nightly, semantic, and fix smoke matrix documented above.
- It copies `autofix-project` into the CI runner temp directory before `fix`, so the self-check proves fix behavior without mutating tracked fixtures.
- It finishes with a clean-worktree assertion to catch accidental tracked fixture edits.

Consumer projects should later get their own reusable workflow/template that installs `go-review` and points at their project-specific `go-review.yaml`.

## Traceability

- Product: `docs/product/go-code-quality-governance.md#回归门禁`
- Backend: `docs/backend/rule-engine-and-autofix.md`
- Quality: `docs/quality/go-code-quality-baseline.md#门禁分层`
- Stories: STORY-009, STORY-010, STORY-011
- Fixture contract: `testdata/fixtures/regression-gates/README.md`
