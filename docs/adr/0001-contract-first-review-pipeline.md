# ADR-0001 Contract-first review pipeline

Status: accepted  
Date: 2026-05-19

## Context

The authoritative docs define a generic Go code-review orchestration platform rather than a wrapper around one linter. The first implementation slice starts from a documentation-only repository, so the team needs stable boundaries before adding broad adapters or fix behavior.

Sources:

- `docs/README.md` platform boundary: adapter access, config loading, orchestration, result normalization, fix transactions, reports, and gates.
- `docs/backend/rule-engine-and-autofix.md` adapter, pipeline DAG, normalized result, and safe-fix contracts.
- `docs/quality/go-code-quality-baseline.md` local, PR/CI, main, and nightly gate layering.
- `.omx/plans/ralplan-docs-implementation-20260519T091728Z.md` recommends Option B: contract-first vertical core, then grouped capability lanes.

## Decision

Implement the first executable surface as a contract-first vertical core:

1. Keep the CLI thin and delegate orchestration to engine/config/pipeline packages.
2. Treat built-ins such as `go.format` as adapters, not as pipeline-core branches.
3. Use one fixture YAML contract for local, ci, main, and nightly profiles so local and CI gates cannot drift silently.
4. Store fixture projects and golden reports under `testdata/fixtures/regression-gates/` for early smoke/integration tests.
5. Defer provider-specific CI and PR-comment integrations until the report artifact contract is stable.

## Consequences

- Early tests can prove profile selection, adapter IDs, gate status, and artifact paths without requiring every production adapter.
- The implementation remains tool-agnostic while still proving concrete behavior with `cmd`, `go.format`, and `go test` adapter IDs.
- Future code lanes must update the fixture contract if config keys or report JSON shape intentionally change.
- CI provider setup remains portable documentation until a provider is selected.

## Rejected alternatives

- **Story-order waterfall:** simple to track but delays profile and fixture feedback until late stories.
- **CLI-only local MVP:** fast user-visible command but risks local-only assumptions leaking into core boundaries.
