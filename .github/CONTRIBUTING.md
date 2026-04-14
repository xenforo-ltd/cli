# Contributing

Thank you for your interest in contributing to the XenForo CLI.

We accept pull requests that fix bugs or implement feature requests labeled
`help wanted`.
We encourage new issues for all other contributions.
For security concerns, follow our [private disclosure policy](SECURITY.md) instead.

## Notes

- Before opening an issue, check that one does not already exist for the same problem or feature.
- Open a bug report if the CLI does not work as expected.
- Open a feature request to propose a new feature or improvement.
- Open a pull request to fix a bug or implement a feature request labeled [`help wanted`](https://github.com/xenforo-ltd/cli/labels/help%20wanted).
- Do not open an issue for support. Use the [development discussions forum](https://xenforo.com/community/forums/xenforo-development-discussions.34/) instead.
- Do not open a feature pull request unless the issue is labeled `help wanted`.
- Keep each pull request focused on one bug or feature.

## Development

### Building

1. Install Go.
2. Clone the repository.
3. Run `make` to build the binary.
4. Run `dist/xf` to use the built binary.

### Tips

- Use `make fmt` to format the code.
- Use `make lint` to lint the code.
- Use `make test` to test the code.

Contributions to this project are released to the public under the [project's
open source license](../LICENSE).

Please note that this project adheres to a [Contributor Code of
Conduct](./CODE_OF_CONDUCT.md). By participating in this project you agree to abide
by its terms.

## Project structure

```text
xf/
├── cmd/
│   └── xf/            # CLI entry point
├── internal/          # Command execution and shared packages
├── scripts/           # Install/test scripts
├── go.mod             # Go module definition
├── go.sum             # Go module sums
├── Makefile           # Build automation
```

### Package boundaries

`cmd/xf` is the process entry point: it handles signals and exits with the
status `cli.Run` returns. `internal/cli` builds the Cobra command tree, binds
flags, dispatches commands, and renders errors and exit codes. It delegates
execution to `internal/commands/<command>` using contexts, arguments, and concrete
options.

Command packages do not import one another. Each subcommand has its own action
file in its parent's package. Helpers used by multiple actions belong in
separate responsibility-named files. Code used by different command families
belongs outside `internal/commands`.

### Shared responsibilities

| Package | Responsibility |
| --- | --- |
| `internal/auth` | OAuth and credential-source selection, storage, and token lifecycle. |
| `internal/build` | Add-on release and JavaScript build execution. |
| `internal/cache` | Downloaded archives, metadata, integrity checks, and cache storage. |
| `internal/clierror` | Error classification, usage markers, hints, cancellation, and child exit status. |
| `internal/cli` | Command tree, flag binding, dispatch, and process-level error reporting. |
| `internal/completion` | Shell completion candidates for licenses, downloads, cache entries, and contexts. |
| `internal/config` | CLI configuration and its defaults. |
| `internal/customerapi` | Authenticated access to the XenForo customer API. |
| `internal/devoutput` | Conversion of prepared development output to release XML. |
| `internal/downloads` | Licensed product selection, acquisition, and cached archive extraction. |
| `internal/environment` | Environment files, Compose operations, and host/container execution. |
| `internal/extract` | Safe extraction of XenForo archives. |
| `internal/passthrough` | Optional installation arguments and shared tool invocation semantics. |
| `internal/setup` | Environment setup reused by initialization and worktree creation. |
| `internal/stream` | Stream progress reporting. |
| `internal/ui` | Terminal rendering, status output, and subprocess progress presentation. |
| `internal/version` | CLI build metadata, including the existing linker-injected values. |
| `internal/xf` | Installation discovery, installed XenForo versions, and installation metadata. |

Export APIs for actual production consumers outside the package, not to expose
implementation details to tests. Serialization and interface methods retain the
visibility their protocols require.
