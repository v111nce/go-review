# Release Process

`go-review` publishes from Git tags. A release tag builds stamped binaries for Linux, macOS, and Windows, then attaches them to a GitHub Release with SHA-256 checksums.

## Version source

The public module path is:

```txt
github.com/v111nce/go-review
```

Release tags should use semantic versions such as `v0.1.0`. Consumer projects can then pin either:

```bash
go install github.com/v111nce/go-review/cmd/go-review@v0.1.0
```

or a downloaded binary from the GitHub Release page.

## Release workflow

`.github/workflows/release.yml` runs on tags matching `v*.*.*` and can also be started manually with `workflow_dispatch` for a dry-run build.

For every release build it:

1. runs `go test ./...` before packaging;
2. cross-compiles `go-review` for `linux/amd64`, `linux/arm64`, `darwin/amd64`, `darwin/arm64`, and `windows/amd64`;
3. stamps `main.version`, `main.commit`, and `main.date` through `-ldflags`;
4. packages each target with `ADOPTION.md` and `QUICKSTART.md`;
5. writes `checksums.txt` with SHA-256 digests;
6. verifies the packaged Linux binary reports the tag and commit;
7. publishes the archives to GitHub Releases when the workflow was triggered by a tag, using `gh release create --verify-tag` so the release must point at the pushed tag.

Manual `workflow_dispatch` runs upload the same `dist/*` files as a CI artifact instead of creating a GitHub Release.

## Cut a release

From a clean `main` checkout:

```bash
git status --short --ignored=no
go test ./...
git tag v0.1.0
git push origin main --tags
```

After GitHub Actions finishes, verify:

```bash
go install github.com/v111nce/go-review/cmd/go-review@v0.1.0
go-review version
```

Release binaries built by the workflow should include `version=v0.1.0` and the tagged commit. A `go install` source build can report Go build-info module metadata when available, but it does not receive the workflow ldflags stamp.

## Local packaging smoke

The workflow build script can be mirrored locally with one target when changing release behavior:

```bash
VERSION=v0.1.0-test
COMMIT=$(git rev-parse HEAD)
DATE=$(date -u +%Y-%m-%dT%H:%M:%SZ)
go build -trimpath \
  -ldflags "-s -w -X main.version=${VERSION} -X main.commit=${COMMIT} -X main.date=${DATE}" \
  -o /tmp/go-review ./cmd/go-review
/tmp/go-review version
```
