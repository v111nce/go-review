# Consumer Project Adoption Guide

This guide explains how another Go repository adopts `go-review`. The framework repository's own CI is dogfooding; consumer projects should own their own `go-review.yaml` and workflow.

## Adoption model

A consumer project keeps three things in its own repository:

1. A project-local `go-review.yaml` that declares adapters, steps, profiles, and artifacts.
2. A CI workflow that installs `go-review` and runs the `ci` profile.
3. Optional local developer commands for `local` check/fix before pushing.

The framework does not need to know the consumer repository layout beyond `--workdir` and the config file.

## Minimal local commands

From a consumer repository root:

```bash
go-review version
go-review check --config go-review.yaml --profile local --workdir .
go-review fix --config go-review.yaml --profile local --workdir .
go-review check --config go-review.yaml --profile ci --workdir .
go-review check --config go-review.yaml --profile nightly --workdir .
```

Before the tool is published, a local checkout of this framework can run the same config with:

```bash
go run /path/to/go-code-reviewer/cmd/go-review check --config go-review.yaml --profile ci --workdir .
```

## Minimal `go-review.yaml`

See [`../../examples/consumer-go-project/go-review.yaml`](../../examples/consumer-go-project/go-review.yaml) for a copyable starter config. It includes:

- `go.format` as a safe local fixer/checker.
- `go.test` through the generic `cmd` adapter.
- `local`, `ci`, and `nightly` profiles.
- artifact output under `artifacts/go-review`.

## GitHub Actions template

See [`../../examples/consumer-go-project/.github/workflows/go-review.yml`](../../examples/consumer-go-project/.github/workflows/go-review.yml).

The template intentionally has a placeholder install line:

```bash
go install github.com/OWNER/go-code-reviewer/cmd/go-review@v0.1.0
```

Replace it with the real module path and release tag after publishing. Until then, projects can run the framework from a checked-out path or a private module path.
The template also runs `go-review version` so CI logs record the exact tool build used for the gate.

## Version metadata

`go-review version` prints the tool version, commit, build date, Go runtime, OS, and architecture. Development builds default to `version=dev commit=unknown date=unknown`.

Release builds should stamp these variables:

```bash
go build \
  -ldflags "-X main.version=${VERSION} -X main.commit=${COMMIT} -X main.date=${DATE}" \
  -o go-review ./cmd/go-review
```

Consumer CI should pin a release tag instead of tracking an unversioned branch.

## Framework self-check vs consumer CI

| Check type | Lives in | Purpose |
| --- | --- | --- |
| Framework self-check | This repository | Proves `go-review` itself and its fixtures still work. |
| Consumer CI | Each adopting project | Proves that project's config and codebase pass its chosen quality gate. |

The self-check should remain deterministic and fixture-backed. Consumer CI should reflect the consumer project's actual adapters, severity rules, and profile choices.

## Next adoption hardening steps

- Publish a real module path and versioned install command.
- Add richer reporter artifacts such as SARIF/GitHub Checks when PR annotation becomes required.
