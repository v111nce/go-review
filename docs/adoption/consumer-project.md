# Consumer Project Adoption Guide

This guide explains how another Go repository adopts `go-review`. The framework repository's own CI is dogfooding; consumer projects should own their own `go-review.yaml` and workflow.

## Adoption model

A consumer project keeps three things in its own repository:

1. A project-local `.go-review/go-review.yaml` that declares adapters, steps, profiles, and artifacts. `go-review init` creates this file, and bare `go-review` auto-creates it when missing.
2. A CI workflow that installs `go-review` and runs the `ci` profile.
3. Optional developer commands for `fast` check/fix before pushing.

The framework does not need to know the consumer repository layout beyond `--workdir` and the config file.

## Tooling model

`go-review` is the orchestrator, not a replacement for mature Go tools:

- Use `golangci-lint` as the preferred runner for common lint and formatter checks when a project wants broad lint coverage.
- Use the currently supported `go/analysis` semantic rule kind for simple direct-call bans; add analyzers or external tools for project-specific semantic rules that need deeper AST and type information.
- Keep `go test` as an independent step; `golangci-lint` does not replace test execution, coverage, or race checks.
- Keep `go-review` responsible for profiles, `on_fail` behavior, artifacts, reports, safe fix transactions, and LLM repair context.

## Minimal developer commands

From a consumer repository root:

```bash
go-review version
go-review init         # optional; bare go-review also initializes missing config
go-review              # defaults to check --profile fast
go-review fix          # applies only safe allowed fixes
go-review --profile ci
go-review --profile nightly
```

Before the tool is published, a local checkout of this framework can run the same config with:

```bash
go run /path/to/go-review/cmd/go-review --profile ci --workdir .
```

## Minimal config

The default location is `.go-review/go-review.yaml`; root-level `go-review.yaml` remains supported as a fallback for older examples. See [`../../examples/consumer-go-project/go-review.yaml`](../../examples/consumer-go-project/go-review.yaml) for a copyable starter config. It includes:

- `go.lint` as a safe local formatter/checker backed by `golangci-lint fmt`.
- `go.test` through the generic `cmd` adapter; keep it separate from lint.
- `fast`, `ci`, and `nightly` profiles.
- artifact output under `artifacts/go-review`.
- optional commented adapters for `staticcheck`, `govulncheck`, and `gosec`, with install commands next to each disabled block.
- future-ready path to replace scattered lint/format commands with a `golangci-lint` adapter while keeping `go-review` as the orchestrator.
- `on_fail: continue` on generated steps so formatting, tests, semantic checks, and any enabled optional scanners all run independently and report together.

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

`go-review check` is read-only. `go-review fix --profile fast` is allowed to edit files only when a step opts in with `allow_fix: true` and the adapter declares `fix_safety: safe`; the default uses this for formatting through `go.lint` / `golangci-lint fmt`; safe fixes remain governed by `allow_fix` plus `fix_safety`. Review-only semantic rules report suggestions but do not rewrite code.

## Semantic rule defaults

Running `go-review init` creates:

```text
.go-review/
  go-review.yaml
  semantic/
    default.yaml  # built-in/default semantic rules
    custom.yaml   # team-owned semantic rules
```

Project-wide `exclude` belongs in `.go-review/go-review.yaml`, not in semantic rule files. The generated config does **not** enable project excludes by default; it only includes a commented example. Add paths only when the repository owner intentionally wants them skipped. Once configured, every built-in project scanner that honors project excludes skips those paths, including `go.lint` and `go.semantic`. Example:

```yaml
exclude:
  - generated
  - third_party
```

`default.yaml` and `custom.yaml` only list semantic rules. Built-in rules go under `rules:`:

```yaml
rules:
  - no-direct-os-getenv
```

Team-owned semantic rules go under `rules:` in `.go-review/semantic/custom.yaml`. Supported rule kinds include `no-direct-call` and `max-params`. `no-direct-call` matches direct calls to an imported package function, including aliased imports such as `import f "fmt"` followed by `f.Println(...)`:

```yaml
rules:
#   - id: no-direct-fmt-println
#     kind: no-direct-call
#     package: fmt
#     function: Println
#     message: "不要直接使用 fmt.Println"
#     suggestion: "改用注入的 logger"
```

For function parameter limits, use `max-params` with `max`:

```yaml
rules:
  - id: max-four-params
    kind: max-params
    max: 4
    message: "方法入参不能超过 4 个"
    suggestion: "拆分参数对象或引入配置结构"
```

When enabled, the report rule ID is prefixed with `semantic.`, for example `semantic.no-direct-fmt-println` or `semantic.max-four-params`. This configuration is intentionally limited: it is a supported rule kind, not a general-purpose semantic DSL. The `go.semantic` adapter does not use the adapter `parser` field; built-in rules are configured in `.go-review/semantic/default.yaml`, and `parser` is not a plugin mechanism. A semantic step currently reports the first failing finding for the step, not a full multi-diagnostic stream, and review-only semantic rules do not auto-fix code. Rules such as return counts, body length, context ordering, or import boundaries need additional implementation: add new `go/analysis` analyzers to `go.semantic`, or use external tools via `cmd`. Keep one `semantic` step in `go-review.yaml`; use the top-level project `exclude` list to skip packages or directories that no profile should scan.

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
