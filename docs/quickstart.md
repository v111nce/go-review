# Quickstart: Go Review Pipeline

This project implements a tool-agnostic Go code-review orchestration platform. The first delivery slice focuses on a runnable fast/CI/nightly review pipeline with fixtures and stable evidence.

## Current implementation slice

The current executable slice implements the full first-story set from `.omx/plans/ralplan-docs-implementation-20260519T091728Z.md`:

1. Go module and thin `go-review` CLI bootstrap.
2. Config, adapter registry, command adapter, and minimal `go.format` wrapper.
3. Normalized result/report artifacts.
4. Pipeline DAG/profile execution for `fast`, `ci`, `main`, and `nightly` gates.
5. Example `go.semantic` custom rule adapter.
6. Safe `go.format` autofix with validation rollback evidence.

## Fixture-backed commands

Use the regression fixtures for smoke checks:

```bash
go test ./...
go run ./cmd/go-review --help
go run ./cmd/go-review version
go run ./cmd/go-review check \
  --config testdata/fixtures/regression-gates/configs/go-review.yaml \
  --profile fast \
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
  --profile fast \
  --workdir testdata/fixtures/regression-gates/autofix-project
```

Expected behavior:

- `fast` on `compliant-project` passes.
- `ci` on `violating-project` fails and emits step, adapter, rule/location when available, reason, and artifact path.
- `nightly` can add long-running steps without changing `fast` or `ci` behavior.
- `semantic` on `semantic-violating-project` fails with `semantic.no-direct-os-getenv`.
- `fix --profile fast` applies only `safe` fixes, reruns validation, and rolls back if a later validation step fails.

## Safe fixes

`check` never modifies files. `fix --profile fast` may modify files, but only for steps marked `allow_fix: true` whose adapter declares `fix_safety: safe`; the current default safe fixer is `go.format`/gofmt. After applying a safe fix, `go-review` reruns dependent validation steps and rolls back the edit if validation fails.

## Framework self-check CI

This repository dogfoods `go-review` through `.github/workflows/self-check.yml`.
That workflow is a framework regression guard, not the full consumer-project adoption story:

- It runs `go test ./...` to protect the framework implementation.
- It runs CLI help to protect the user-facing command surface.
- It runs CLI version output so release/debug metadata stays available.
- It runs the same fixture-backed fast, ci, nightly, semantic, and fix smoke matrix documented above.
- It copies `autofix-project` into the CI runner temp directory before `fix`, so the self-check proves fix behavior without mutating tracked fixtures.
- It finishes with a clean-worktree assertion to catch accidental tracked fixture edits.

Consumer projects can start from the checked-in template under `examples/consumer-go-project/.github/workflows/go-review.yml`; each adopting repository should still own its project-specific `go-review.yaml`.

## Release packaging

The tag-based release workflow lives at `.github/workflows/release.yml`. It runs `go test ./...`, cross-compiles stamped `go-review` binaries for Linux, macOS, and Windows, writes `checksums.txt`, verifies packaged version metadata, and publishes GitHub Release assets for tags such as `v0.1.0`. See [Release Process](release.md).

## Consumer project adoption

For another Go repository, start from:

- [Consumer Project Adoption Guide](adoption/consumer-project.md)
- [`examples/consumer-go-project/go-review.yaml`](../examples/consumer-go-project/go-review.yaml)
- [`examples/consumer-go-project/.github/workflows/go-review.yml`](../examples/consumer-go-project/.github/workflows/go-review.yml)

The example proves that `go-review` can be invoked from outside its own module by passing a consumer-owned config and `--workdir`.

## Traceability

- Product: `docs/product/go-code-quality-governance.md#回归门禁`
- Backend: `docs/backend/rule-engine-and-autofix.md`
- Quality: `docs/quality/go-code-quality-baseline.md#门禁分层`
- Stories: STORY-009, STORY-010, STORY-011
- Fixture contract: `testdata/fixtures/regression-gates/README.md`
