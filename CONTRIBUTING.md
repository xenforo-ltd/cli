# Contributing to xf

Thanks for contributing.

## Development setup

1. Install Go.
2. Clone the repository.
3. Run `go test ./...`.
4. Run `go test -race ./...`.
5. Run `go vet ./...`.
6. Ensure formatting is clean with `gofmt -w`.

## Pull requests

1. Keep PRs focused and small where possible.
2. Include tests for behavior changes.
3. Update docs/help text when flags or behavior change.
4. Ensure CI is green across macOS, Linux, and Windows.

## Commit quality

1. Avoid unrelated refactors in feature/fix PRs.
2. Keep user-facing output clear and actionable.
3. Preserve backward compatibility unless explicitly discussed.

## Reporting issues

Use the issue templates and include:
1. `xf version --json`
2. Host OS and architecture
3. Exact command and output
4. Steps to reproduce
