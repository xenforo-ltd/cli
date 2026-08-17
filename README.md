# XenForo CLI (`xf`)

A command-line tool for provisioning and managing XenForo development environments with Docker.

## Requirements

- Go
- Docker with Docker Compose plugin
- Git
- System keychain
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

## Install for Local Use

```bash
# Installs into $(go env GOBIN) or $(go env GOPATH)/bin
go install .

# Verify
xf version
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

# .env overrides (file + inline; inline wins)
xf init ./my-project \
  --env-file ./my.env \
  --env XF_TITLE="My Site"

# Note: init defaults XF_DEBUG=1 and XF_DEVELOPMENT=1.
# You can override either key via --env-file/--env.
```

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
--non-interactive      # Disable prompts (for CI/automation)
-v, --verbose          # Enable verbose output
```

## Data and Paths

- Config: `~/.config/xf/config.json`
- Cache: `~/.config/xf/cache`
- Project metadata file: `.xf.json`
- OAuth token storage: system keychain service `xf`

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

- OAuth tokens are stored only in the system keychain (no plaintext file fallback).
- PKCE is used for OAuth authorization flow security.
- Tokens are refreshed automatically when needed.

## Building for Release

```bash
# Build with embedded version information
make release VERSION=1.0.0
```
