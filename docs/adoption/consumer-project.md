# Consumer Project Adoption Guide

This guide explains how another Go repository adopts `go-review`. The framework repository's own CI is dogfooding; consumer projects should own their own `go-review.yaml` and workflow.

## Adoption model

A consumer project keeps three things in its own repository:

1. A project-local `go-review.yaml` that declares adapters, steps, profiles, and artifacts.
2. A CI workflow that installs `go-review` and runs the `ci` profile.
3. Optional developer commands for `fast` check/fix before pushing.

The framework does not need to know the consumer repository layout beyond `--workdir` and the config file.

## Minimal developer commands

From a consumer repository root:

```bash
go-review version
go-review              # defaults to check --profile fast and discovers go-review.yaml
go-review fix          # applies only safe allowed fixes
go-review --profile ci
go-review --profile nightly
```

Before the tool is published, a local checkout of this framework can run the same config with:

```bash
go run /path/to/go-review/cmd/go-review --profile ci --workdir .
```

## Minimal `go-review.yaml`

See [`../../examples/consumer-go-project/go-review.yaml`](../../examples/consumer-go-project/go-review.yaml) for a copyable starter config. It includes:

- `go.format` as a safe local fixer/checker.
- `go.test` through the generic `cmd` adapter.
- `fast`, `ci`, and `nightly` profiles.
- artifact output under `artifacts/go-review`.

## Reports

Every `check` and `fix` run writes deterministic reports. By default, reports are written next to the config under `.go-review/reports/`; use `--report-dir <dir>` to override the location.

```txt
.go-review/
  reports/
    latest.md        # human-readable summary
    latest.llm.md    # repair context designed to paste into an LLM
    latest.json      # machine-readable result contract
    runs/            # timestamped copies of each run
```

`latest.md` answers "did it pass, what failed, can it auto-fix, and what do I do next?". `latest.llm.md` repeats the same deterministic results in a repair-oriented prompt shape with file/line/column, rule, message, suggestion, fix safety, and artifact paths. No LLM is required to generate either report.

## Safe fix behavior

`go-review check` is read-only. `go-review fix --profile fast` is allowed to edit files only when a step opts in with `allow_fix: true` and the adapter declares `fix_safety: safe`; the default example uses this for formatting/gofmt. Review-only semantic rules report suggestions but do not rewrite code.

## GitHub Actions template

See [`../../examples/consumer-go-project/.github/workflows/go-review.yml`](../../examples/consumer-go-project/.github/workflows/go-review.yml).

The template now uses the chosen public module path with a placeholder release tag:

```bash
go install github.com/v111nce/go-review/cmd/go-review@v0.1.0
```

Replace `v0.1.0` with the real release tag after publishing. Until then, projects can run the framework from a checked-out path or a private module path.
The template also runs `go-review version` so CI logs record the tool build used for the gate. Source installs expose module/build-info metadata when available; release binaries add explicit stamped metadata.

## Version metadata

`go-review version` prints the tool version, commit, build date, Go runtime, OS, and architecture. Development builds default to `version=dev commit=unknown date=unknown`, but source installs can fall back to Go build-info metadata when available.

Release builds should stamp these variables:

```bash
go build \
  -ldflags "-X main.version=${VERSION} -X main.commit=${COMMIT} -X main.date=${DATE}" \
  -o go-review ./cmd/go-review
```

Consumer CI should pin a release tag instead of tracking an unversioned branch. Downloaded release binaries are available from the tag workflow described in [Release Process](../release.md).

## Framework self-check vs consumer CI

| Check type | Lives in | Purpose |
| --- | --- | --- |
| Framework self-check | This repository | Proves `go-review` itself and its fixtures still work. |
| Consumer CI | Each adopting project | Proves that project's config and codebase pass its chosen quality gate. |

The self-check should remain deterministic and fixture-backed. Consumer CI should reflect the consumer project's actual adapters, severity rules, and profile choices.

## Next adoption hardening steps

- Publish a real release tag and keep the versioned install command pinned.
- Add richer reporter artifacts such as SARIF/GitHub Checks when PR annotation becomes required.
