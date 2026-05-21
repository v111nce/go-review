# Consumer Go Project Example

This directory is a minimal example of how a separate Go project adopts `go-review`.
It is intentionally small: the goal is to show project-local configuration and CI wiring, not to test every adapter.

## Files

- `go-review.yaml` — project-owned pipeline config.
- `.github/workflows/go-review.yml` — copyable GitHub Actions workflow for a consumer repository.
- `cmd/app` and `internal/app` — tiny Go module used by the example.

## Local adoption flow from this framework repository

```bash
go run ../../cmd/go-review check --config go-review.yaml --profile fast --workdir .
go run ../../cmd/go-review fix --config go-review.yaml --profile fast --workdir .
```

## Consumer repository flow after release

```bash
go install github.com/v111nce/go-review/cmd/go-review@v0.1.0
go-review check --config go-review.yaml --profile ci --workdir .
```

Replace `v0.1.0` with the real release tag once published.
