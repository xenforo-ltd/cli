# Agent guide

Operational rules for working in this repository. Keep this file short and
limited to things that are stable and easy to get wrong.

## Verification

Run before proposing a change as complete:

```bash
make lint   # go vet + gofmt check
make test   # go test ./...
make build  # compile the binary
```

`make all` runs all three. Do not claim work is verified without running them.

## Argument forwarding

`php`, `php-debug`, `composer`, `compose`, `exec` and `debug` wrap another tool
and set `DisableFlagParsing`. For these commands:

> **`xf`'s own flags go before the command name. Everything after it belongs to
> the wrapped tool.**

```bash
xf php -v                       # runs: php -v
xf composer outdated --direct   # runs: composer outdated --direct
xf --verbose php my-script.php  # --verbose is consumed by xf
xf php -- -v                    # a leading -- is accepted and removed
```

Consequences worth remembering:

- `xf php --help` shows **PHP's** help. Use `xf help php` for `xf`'s help.
- `xf php --verbose` forwards `--verbose` to PHP; it does not enable `xf`
  verbosity. Put the flag before the command name.
- `rootCmd` sets `TraverseChildren` so pre-command globals still work. Removing
  it silently breaks `xf --verbose php ...`. It is covered by a test.

All other commands parse flags normally and accept them in **any** position.
`logs` is deliberately not a passthrough command, because `--follow` belongs to
`xf`. Do not add `DisableFlagParsing` to a command unless it forwards everything
to an external tool.

## Command routing

Two distinct routes exist, with different flag semantics. This is intentional.

1. **Wrapper commands** (`php`, `composer`, …) are cobra commands that forward
   their arguments. They follow the rule above.
2. **Direct XenForo commands** (`xf xf-dev:import`, `xf list`) are routed in
   `Execute` before cobra parses anything, and are forwarded verbatim.

Known limitation: the direct route is only taken when the first argument is not
a flag, so a global flag cannot be combined with it.

```bash
xf xf-dev:import                    # works
xf -v xf-dev:import                 # prints xf's help, exits 0, runs nothing
xf --verbose debug xf-dev:import    # use a wrapper command instead
```

The silent success in the second form is a real trap: it looks like the command
ran. Treat it as a known bug to fix separately, not as intended behaviour.

## Error reporting

`Execute` owns error output; cobra's own printing is silenced.

- Usage is printed **only** for invocation mistakes, which are marked with
  `usageError`.
- Runtime failures (Docker not reachable, a script failing) print one concise
  line with no usage block.
- When adding a validation check inside `RunE` that means "the command was typed
  incorrectly", wrap it with `newUsageError(...)` so usage is shown.

Do not classify errors as usage errors by sentinel alone. `ErrInvalidInput` also
covers user cancellation and environment state, where usage output is unhelpful
or misleading. Audit each site individually.

## Conventions

- Leaf commands should set `cobra.NoArgs` unless they take positional arguments.
- A parent command with subcommands needs **both** `cobra.NoArgs` and a `RunE`
  that prints help. Without `RunE`, cobra skips the parent's `Args` validator and
  silently prints help for an unknown subcommand.
- Commit messages follow Conventional Commits; breaking changes use `!` and a
  `BREAKING CHANGE:` footer.
