# XenForo CLI (`xf`)

A command-line tool for provisioning and managing XenForo development environments with Docker.

## Requirements

- Go
- Docker with Docker Compose plugin
- Git
- System keychain (optional when using file storage or `XF_TOKEN`)
  - macOS Keychain
  - Windows Credential Manager
  - Linux Secret Service

## Quick Start

```bash
# Build locally
make build

# Authenticate
./xf auth login

# Initialize a project
./xf init ./my-project

# Show all built-in xf commands
./xf --help
```

## Install

```bash
# Homebrew (macOS and Linux)
brew install xenforo-ltd/tap/xf-cli

# macOS and Linux, installs into ~/.xf/bin
curl -fsSL https://raw.githubusercontent.com/xenforo-ltd/cli/main/scripts/install.sh | bash
```

```powershell
# Windows, installs into %USERPROFILE%\.xf\bin
irm https://raw.githubusercontent.com/xenforo-ltd/cli/main/scripts/install.ps1 | iex
```

Update an installer-managed copy with `xf self-update`, or with `brew upgrade`
if you installed it with Homebrew.

To install from source into `$(go env GOBIN)` or `$(go env GOPATH)/bin`:

```bash
go install ./cmd/xf
```

## Command Routing

`xf` supports two command types:

1. Built-in commands such as `xf init`, `xf up`, `xf auth login`.
2. XenForo commands: if the first token is not a built-in command, `xf` forwards it to XenForo inside Docker.
   If Docker config is not initialized (no `compose.yaml`), `xf` falls back to local execution as `php cmd.php <args...>`.

Use this to discover available XenForo commands:

```bash
# Run from a XenForo directory (or set XF_DIR)
xf list

# Run a XenForo command directly
xf xf-dev:import
```

If you are not in a XenForo directory, set `XF_DIR` to a directory that contains `src/XF.php`.

### Passing arguments to wrapped tools

`php`, `php-debug`, `composer`, `compose`, `exec` and `debug` wrap another tool.
For these commands:

> **`xf`'s own flags go before the command name. Everything after it belongs to
> the wrapped tool.**

```bash
# Flags after the command reach the tool
xf php -v                       # runs: php -v
xf composer outdated --direct   # runs: composer outdated --direct

# xf's own flags go first
xf --verbose php my-script.php

# A leading -- is accepted and removed, for habit or clarity
xf php -- -v
```

Because everything after the command is forwarded, `xf php --help` shows PHP's
help, not `xf`'s. Use `xf help php` (or `xf help composer`, and so on) to read
`xf`'s help for a wrapped command.

All other commands parse flags normally and accept them in any position. `xf logs
--follow` and `xf logs xf --follow` are equivalent, because `--follow` belongs to
`xf` rather than to a wrapped tool.

### Known limitation

Global flags cannot currently be combined with the direct XenForo route, because
that route is only taken when the first argument is not a flag:

```bash
xf xf-dev:import        # works
xf -v xf-dev:import     # prints xf's help and exits 0 without running anything
```

Use a wrapped command if you need this, for example `xf --verbose debug xf-dev:import`.

## CLI Usage

### Authentication

Choose the credential source that suits your environment:

- **Keychain** (default): browser login stores tokens in the system keychain.
- **File**: set `auth.storage` to `file` in `config.json`, or set `XF_AUTH_STORAGE=file` for each invocation. Login, refresh, and logout then use this store.
- **Environment**: a non-empty `XF_TOKEN` overrides either store. Supply a raw OAuth access token, without a `Bearer ` prefix. The token is kept in memory and sent to the configured OAuth base URL (`oauth.base_url` / `XF_OAUTH_BASE_URL`).

To select file storage permanently, merge this setting into your configuration:

```json
{
  "auth": {
    "storage": "file"
  }
}
```

Or select it through the environment:

```bash
export XF_AUTH_STORAGE=file
xf auth login
xf auth status
```

Credentials are stored in `auth.json` beside `config.json` (also when using `--config`). Without a custom configuration path, this is the OS user configuration directory under `xf`. The file includes access and refresh tokens, expiry, and the OAuth base URL. Unix files are created with mode `0600`; overly broad permissions and symlinks are rejected for normal use. Login checks storage before opening the browser. Logout can read and delete a regular file with overly broad permissions so its tokens can be revoked, but it never follows a credential-file symlink. Refresh replaces the file atomically on Unix. Windows replacement uses the OS rename operation and does not offer the same atomicity guarantee. Keep this file private and out of version control. Login and status display the resolved credential-file path. Changing stores does not copy or remove credentials in the previous store; log in to populate the selected store.

For CI, inject `XF_TOKEN` using your CI secret settings, then run commands normally:

```bash
xf --no-interaction download --license … --download xenforo --version …
```

`XF_TOKEN` must contain an existing OAuth access token; this option does not issue tokens or provide unattended login. Unset or empty values use the selected persistent store. Whitespace or control characters are rejected. Environment tokens cannot be refreshed by the CLI: replace the CI secret when it expires. While `XF_TOKEN` is active, `auth login`, `auth refresh`, and `auth logout` explain how to manage it externally; unset it to manage stored credentials. Unsetting a token does not revoke it on the server. An invalid environment token never falls back to stored credentials.

`auth status` reports the credential source without exposing the token. For environment tokens it validates with the server; JSON validity and expiry fields are `null` when unknown. `auth status --json` also returns JSON for credential errors, with a stable `reason` such as `not_authenticated`, `store_unavailable`, `configuration_mismatch`, `invalid_credentials`, or `storage_error`. A normal logged-out state has no `error` field. `doctor` checks the selected credential source for usability, preflighting writable persistent stores before reporting authentication healthy, reports storage failures separately from being logged out, and reports environment-token presence without claiming server validity.


```bash
# Log in
xf auth login

# Log in with a custom browser callback timeout (seconds)
xf auth login --timeout 600

# Check auth status
xf auth status
xf auth status --json

# Refresh token
xf auth refresh

# Log out and revoke tokens
xf auth logout
```

### Initialize

```bash
# Interactive init
xf init ./my-project

# Interactive flow notes:
# - XenForo core is always installed
# - You choose only additional products
# - Core version picker shows the latest 10 versions + manual entry
# - A final review screen lets you edit all values before work starts

# Non-interactive init
xf init ./my-project \
  --license 02306C2650 \
  --version 2030871 \
  --products xfmg,xfes \
  --admin-user admin \
  --admin-password secret \
  --admin-email admin@example.com

# Existing directory mode
xf init ./existing-xf-project --existing
xf init ./existing-xf-project --existing --up

# Composer dependencies are installed automatically when the target
# tracks a composer.json (repository checkouts). Release packages ship
# vendor/ prebuilt and are skipped.

# .env overrides (file + inline; inline wins)
xf init ./my-project \
  --env-file ./my.env \
  --env XF_TITLE="My Site"

# Note: init defaults XF_DEBUG=1 and XF_DEVELOPMENT=1.
# You can override either key via --env-file/--env.
```

Init and worktree setup build JavaScript for XF and add-ons with a `build.json`,
even without starting containers. Release packages omit this file and are skipped.
Use `--skip-build-js` to skip these builds; normal `xf up` does not build JS.

### Upgrade

```bash
# Interactive upgrade
xf upgrade ./my-project

# Upgrade to a specific version
xf upgrade ./my-project --version 2030971

# Skip running xf:upgrade
xf upgrade ./my-project --version 2030971 --skip-upgrade
```

### Download Packages

```bash
# List downloads for a license
xf download --license 02306C2650

# List versions for a product
xf download --license 02306C2650 --download xenforo

# Download a specific version
xf download --license 02306C2650 --download xenforo --version 12345

# Force re-download even if cached
xf download --license 02306C2650 --download xenforo --version 12345 --force
```

### Cache

```bash
xf cache list
xf cache list --license 02306C2650
xf cache list --json
xf cache purge --license 02306C2650
xf cache purge --all
xf cache path
```

### Docker Environment

```bash
# Lifecycle
xf up
xf down
xf reboot

# Status and logs
xf ps
xf logs
xf logs --follow

# Docker Compose passthrough
xf compose ps
xf compose exec xf mysql -u root

# Exec into a service
xf exec xf ls -la
```

### Database

Open the environment's MySQL database in TablePlus. On macOS with OrbStack
the container's `orb.local` hostname is used, so no ports need publishing.

```bash
# Open in TablePlus
xf db

# Print the connection URL instead
xf db --print-url

# Open the database client inside the container (any platform)
xf db shell

# Forward arguments to the client
xf db shell -e 'show tables;'
```

`xf database` is an alias. Only MySQL environments are supported for now.

### Worktrees

A worktree is a second checkout of the same repository on its own branch or
workspace, with its own Docker containers and database. Worktrees are created
alongside the source checkout: `~/Sites/main` gains
`~/Sites/main.worktrees/<name>`.

The source repository decides the backend. A repository that uses Jujutsu —
including one colocated with Git — gets a Jujutsu workspace named from
`<branch>`; no bookmark is created or moved. A plain Git repository gets a Git
worktree and branch. `--base` accepts the selected backend's revision syntax: a
Git ref or a Jujutsu revset.

By default `create` clones the source environment — database, `data/` and
`internal_data/` — and points the cloned board at its own URL, labelling its
title with the worktree name.

A worktree's name comes from the branch's last segment, lowercased with
punctuation turned into dashes: `dev/xfs/user-state-ladder-redesign` becomes
`user-state-ladder-redesign`. The same name is used for the directory, the
Docker instance and the URL, `https://user-state-ladder-redesign.xf.local`. If
the name is already taken — by another worktree, such as `dev/24x/feature` and
`dev/xfs/feature`, by the source checkout's own instance, or by an existing
Docker environment — a number is added:
`feature-2`, `feature-3`. Pass `--instance` to choose the Docker instance name
yourself.

`path` and `remove` accept either the branch or the worktree's name.

`create --json` intentionally does not run setup: setup writes progress to
stdout, which would corrupt the JSON document. It therefore requires
`--no-setup`, and the accepted invocation writes exactly one JSON document to
stdout.

```bash
# Create a worktree and set up its environment
xf worktree create dev/24x/feature

# Base the new checkout on something other than the current HEAD
xf worktree create dev/24x/feature --base main

# Install a clean forum instead of cloning the source environment
xf worktree create dev/24x/feature --fresh

# Create the worktree without setting anything up
xf worktree create dev/24x/feature --no-setup

# Print the created worktree as JSON (requires --no-setup)
xf worktree create dev/24x/feature --no-setup --json

# Configure the environment but leave the containers stopped
xf worktree create dev/24x/feature --no-up

# List worktrees (this project / all known projects)
xf worktree list
xf worktree list-all

# Machine-readable output
xf worktree list --json

# Print the path of a worktree (bare output, shell-substitution safe)
cd "$(xf worktree path dev/24x/feature)"
cd "$(xf worktree path feature)"

# Remove a worktree and its containers and volumes
xf worktree remove dev/24x/feature

# Remove the worktree but leave its containers and volumes running
xf worktree remove dev/24x/feature --keep-containers

# Drop registry entries for worktrees that no longer exist
xf worktree prune
```

`remove` refuses a worktree that holds uncommitted changes or commits that
exist on no remote, and lists what would be lost. The files `create` generates
(`compose*.yaml`, `.env`, `.dockerignore`) do not count as changes. Passing
`--force` overrides both checks, so reach for it only once you have read what
the refusal reported.

Because cloning copies an installed database, `--no-up` and `--fresh` both skip
it: `--fresh` installs a clean forum instead, while `--no-up` leaves the
worktree with no database until you start it and install one yourself.

### Add-on Builds

Run these commands from the XenForo installation or a directory beneath it,
or set `XF_DIR` to the installation path. The add-on ID is optional: without it,
the add-on is inferred from the enclosing directory under `src/addons`, falling
back to `XF`.

```bash
# Build a release ZIP and hash manifest
xf build Vendor/AddOn

# Reuse installed dependencies
xf build Vendor/AddOn --skip-composer

# Override the ZIP timestamp with a Unix timestamp supplied by CI
BUILD_DATE=1767225600 xf build Vendor/AddOn

# Build JavaScript without creating a release
xf build js Vendor/AddOn

# Rebuild JavaScript when inputs change
xf build js Vendor/AddOn --watch
```

The add-on's optional `build.json` controls file selection, minification, rollups,
and release commands. Without it, the build root is the add-on directory, all
eligible files are included, and no JavaScript or exec tasks run. Built-in
exclusions still apply. Hidden files and directories are excluded unless whitelist
patterns explicitly name their dot-prefixed components. For example, `.htaccess`
includes that file, `.config/**` includes visible files under `.config`, and
`**` alone does not include hidden paths. Blacklist rules and reserved build-file
exclusions still apply. Additional files and JavaScript inputs are looked up in
the build root's `_files` directory first, then in the build root itself.
Additional entries may be literal file or directory paths, or glob patterns.

Minifying a script writes its `*.min.js` companion and an external
`*.min.js.map` source map, and links the map from the minified file. Maps do not
embed original sources (`sourcesContent`); the unminified sources ship alongside
them. Rollups concatenate their inputs as plain text without stripping comments.
Wildcard rollup inputs exclude `.min.js` files and outputs of later rollups;
list a file explicitly to include it. Earlier rollup outputs can be used for
chaining. A bundle's own output and minified companion are always excluded
from its inputs. Minification always runs after rollups: a `minify` pattern that
matches the bundle produces its `*.min.js` companion and a map that points back
at the readable bundle.

Builds check only the `addon.json` fields they use: a usable `version_string`
and a valid `composer_autoload` path when provided. Full metadata validation is
deferred. The add-on's `addon.json` and a declared Composer `installed.json` are
always included, regardless of whitelist and blacklist rules; additional-file
overrides still apply. The Composer file must be available even with
`--skip-composer`. Release builds export XML
from the add-on's prepared `_output` directory into `_data`, after Composer and
before JavaScript tasks. Non-empty type directories require `_metadata.jsonl`,
and listed files must match their recorded hashes. Unknown types, unknown JSON
fields, and custom PHP export handlers are rejected. JavaScript-only builds do
not export data or require `_output` or `_data`.

Every ZIP entry uses the build start time by default, including generated files
and instructions. Set `BUILD_DATE` to a Unix timestamp to override it and
reproduce a previous archive. The timestamp must fall between 1980-01-01 and
2106-02-07 06:28:15 UTC. Source modification times do not affect the ZIP bytes.

`hash_exclude` is an optional array of paths or globs omitted from `hashes.json`
without removing files from the ZIP. Patterns match upload destinations relative
to the build root; additional files mapped outside that root use their
installation-relative destination. Use `**/index.html` to exclude that filename
at any depth. A literal directory path also excludes its descendants.

Releases are written to the add-on's `_releases` directory; the generated hash
manifest is written to `hashes.json` in the add-on directory. Composer uses the
Docker environment when configured, or the host otherwise.

Configured `exec` commands run on the host unless the first argument is `@php`,
`@composer`, or `@xf`. These aliases use the Docker environment when
`compose.yaml` is present, running in the `xf` service or a one-off container,
and run on the host otherwise. `@php` and `@composer` run in the add-on
directory, like ordinary commands. `@xf` runs `php cmd.php` in the installation
root and accepts XenForo command arguments, not `xf` CLI options. Aliases are
recognized only as the exact first argument. Docker failures are reported and
never retried on the host.

```json
{
  "exec": [
    ["@php", "build.php", "{version_string}"],
    "@composer dump-autoload --no-dev",
    ["@xf", "xf-dev:rebuild-caches"]
  ]
}
```

### PHP / Composer / Debug

```bash
# PHP and Composer
xf php -v
xf composer install
xf composer outdated --direct

# XenForo command with XDebug enabled
xf debug xf-dev:import

# PHP with XDebug enabled
xf php-debug -v

# Read xf's help for a wrapped command
xf help php
```

`xf composer` uses the Docker environment when `compose.yaml` is present,
executing in the running `xf` service or starting a one-off container otherwise.
Without `compose.yaml`, it runs the host's Composer in the XenForo directory.
Docker failures are reported rather than retried on the host.

### Other Commands

```bash
xf licenses
xf doctor
xf self-update
xf self-update --check-only
xf version
xf version --json
xf version --short
```

### Shell Completion

```bash
# Example: zsh
xf completion zsh
```

### Global Flags

Available on all built-in commands:

```bash
-n, --no-interaction   # Disable prompts (for CI/automation)
-v, --verbose          # Enable verbose output
-c, --config <path>    # Use an alternative config file
```

## Data and Paths

- Config: `~/.config/xf/config.json`
- Cache: `~/.config/xf/cache`
- Project metadata file: `.xf.json`
- OAuth token storage: system keychain service `xf` by default, or `auth.json` beside the configuration file when file storage is selected.

## Development Commands

### Make

```bash
make build      # Build the binary
make run        # Run without building (go run)
make test       # Run all tests
make test-v     # Run tests with verbose output
make test-cover # Run tests with coverage
make fmt        # Format code
make vet        # Check for common mistakes
make tidy       # Update dependencies
make clean      # Remove built binary
make all        # Format, vet, test, and build
```

### Go Directly

```bash
# Build
go build -o xf .

# Run without building
go run . --help
go run . version

# Tests
go test ./...
go test ./... -v
go test ./... -cover
```

## Project Structure

```text
xf/
├── cmd/                    # CLI commands
├── internal/
│   ├── api/                # XenForo API client
│   ├── auth/               # OAuth and keychain integration
│   ├── cache/              # Download cache management
│   ├── config/             # Config and environment settings
│   ├── dockercompose/      # Docker Compose runner integration
│   ├── doctor/             # System diagnostics
│   ├── downloads/          # Download orchestration
│   ├── embed/              # Embedded Docker assets
│   ├── errors/             # Structured error types
│   ├── extract/            # Archive extraction
│   ├── selfupdate/         # Self-update logic
│   ├── stream/             # Streaming/progress helpers
│   ├── ui/                 # CLI UI helpers
│   ├── version/            # Build/runtime version info
│   ├── xf/                 # XenForo-specific helpers
│   └── xfcmd/              # XenForo command helpers
├── scripts/                # Install/test scripts
├── main.go                 # Entry point
├── go.mod                  # Go module definition
├── Makefile                # Build automation
└── README.md
```

## Security

- OAuth tokens use the system keychain by default. File storage is an explicit opt-in and stores credentials as plaintext with owner-only file permissions on Unix. On Windows, access is controlled by the containing directory’s ACL; use a private user directory. There is no automatic plaintext fallback.
- PKCE is used for OAuth authorization flow security.
- Stored tokens are refreshed automatically when needed. Environment tokens must be replaced externally when expired or revoked.

## Building for Release

```bash
# Build with embedded version information
make release VERSION=1.0.0
```
