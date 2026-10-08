# Changelog

All notable changes to this project will be documented in this file.

## [0.2.0] - 2026-10-08

### Features

- *(auth)* Support file and environment credentials
- *(auth)* Add JSON status output and a clean unauthenticated state
- *(auth)* Animate the browser login flow
- *(cli)* **Breaking**: Forward all arguments to passthrough commands
- *(cli)* Distinguish cancellation, interrupts and child exit codes
- *(cli)* Pass through the exit code of wrapped commands
- *(cli)* Group commands and style help output
- *(cli)* Improve download and self-update progress output
- *(cli)* Restyle environment command output
- *(db)* Open the environment's database in TablePlus
- *(docker)* Expose the embedded file list
- *(dockercompose)* Add environment-safe exec, project checks and teardown
- *(doctor)* Diagnose configured credential source
- *(doctor)* Render suggestions and fail with a non-zero status
- *(errors)* Add classified, hinted and cancellation errors
- *(errors)* Render errors with hints and concise causes
- *(init)* Install Composer dependencies for existing checkouts
- *(init)* Report .default files written for user-modified configs
- *(init)* Improve the review editor and install summary
- *(licenses)* Restyle license output
- *(ps)* Render a native container table
- *(ui)* Add shared formatting and output helpers
- *(upgrade)* Improve upgrade progress and failure reporting
- *(worktree)* Add repository backends, path resolution and creation
- *(worktree)* Add recursive directory copy
- *(worktree)* Add the worktree registry
- *(worktree)* Add the xf worktree command
- *(worktree)* Render worktree state and restyle output

### Bug fixes

- *(cache)* Validate purge flags and keep path output script-friendly
- *(cli)* Handle more errors in XF directory detection
- *(cli)* Pass flags through to passthrough commands without --
- *(cli)* Report runtime errors without usage
- *(cli)* Reject unexpected arguments on leaf commands
- *(cli)* Classify audited invocation errors as usage errors
- *(cli)* Exit quietly when interrupted
- *(cli)* Scope config initialization to the root command
- *(cli)* Reject an unknown command after a global flag
- *(cli)* Redact forwarded arguments from debug and PHP output
- *(config)* Guard Init against concurrent callers
- *(docker)* Add OCI labels and Orbstack icons to Docker images
- *(docker)* Set `opcache.revalidate_freq` to `0` by default
- *(docker)* Fix line continuation
- *(docker)* Grant containers access to generated and runtime files
- *(dockercompose)* Replay captured stderr when exec fails
- *(download)* Avoid the checksum verification panic
- *(init)* Quote installer arguments
- *(init)* Correct install step counts and mark skipped steps
- *(init)* Persist validation warnings on the review screen
- *(init)* Report install failures once and honour cancellation
- *(release)* Match installers and self-update to the published archives
- *(selfupdate)* Harden against zip-slips
- *(selfupdate)* Build the extracted binary path from a constant name
- *(ui)* Make spinners and progress bars TTY-aware and stoppable
- *(version)* Make --short output script-friendly

### Refactor

- *(api)* Trim unused response fields
- *(cache)* Remove unused cache operations and fields
- *(cli)* Define the root pre-run hook with the command
- *(dockercompose)* Collapse runner command API
- *(doctor)* Remove test-only status helpers
- *(errors)* Remove superseded sentinels
- *(extract)* Remove unused generic archive API
- *(init)* Classify prompts, lookups and cancellations
- *(init)* Rename license option label to licenseLabel
- *(init)* Drop unused environment-provenance map
- *(init)* Merge environment maps with maps.Copy
- *(ui)* Remove unused styles and helpers
- *(ui)* Remove dead helpers and orphaned styles
- *(worktree)* Narrate clone with a single spinner
- *(worktree)* Remove thin exported wrappers
- *(worktree)* Remove unused board retitle helper
- *(xf)* Export the maximum instance name length
- *(xf)* Remove unused environment configuration model
- Remove redundant convenience wrappers
- Remove unreachable helper APIs

### Documentation

- *(auth)* Document credential storage options
- *(cli)* Document command routing and forwarding
- *(github)* Add concise pull request template
- *(readme)* Document worktrees and automatic Composer install
- *(readme)* Document -n/--no-interaction and -c/--config
- *(xdebug)* Fix references to XDebug

### Tests

- *(cmd)* Mark the re-exec helper parameter unused

### Build

- *(goreleaser)* Do not generate package manifests for pre-release versions
- *(make)* Set build date using same format as CI/CD
- *(make)* Disable CGO where sensible and clean up coverage

### Miscellaneous

- *(changelog)* Use git-cliff to manage changelog
- *(codecov)* Set realistic coverage targets
- *(dependabot)* Set Dependabot cooldown for supply chain attack mitigation
- *(dependabot)* Add Compose dependencies to Dependabot
- *(gha)* Harden GHA workflows
- *(gha)* Further restrict top-level permissions
- *(gha)* Update GHA comments to a format Dependabot understands
- *(git)* Ignore local docs directory
- *(go)* Fix legacy syntax
- *(go)* Use declared toolchain for lint
- *(lint)* Use golangci-lint v2.13 for Go 1.27 support
- *(t3)* Add project configuration with scripts and icon
- *(t3)* Downscale project icon
- Publish additional PHP images
- Add SVG project icon
- Minify SVG icon
- Modernize Go stdlib usage
- Track the renamed golangci config in path filters

### Other

- Rename Golangci-lint config
- Tweak WinGet manifest generation
- Move GitHub docs into `.github` directory
- Update security report URL
- Use latest stable Go version for GHA
- Adjust issue template configuration
- Bump Docker dependencies
- Bump charm.land/lipgloss/v2 from 2.0.3 to 2.0.4 in the gomod group
- Bump the github-actions group across 1 directory with 6 updates

## [0.1.7-alpha.7] - 2026-04-23

### Other

- Temporarily disable Homebrew completion generation
- Enable WinGet manifest generation
- Fix GoReleaser GitHub token

## [0.1.7-alpha.6] - 2026-04-23

### Other

- Fix Homebrew completions generation

## [0.1.7-alpha.5] - 2026-04-23

### Other

- Fix linker version metadata
- Use CodeCov repository token
- Re-enable notarization

## [0.1.7-alpha.4] - 2026-04-22

### Other

- Fix attestation permissions
- Adjust GoReleaser config

## [0.1.7-alpha.3] - 2026-04-22

### Other

- Slight tweaks to Makefile
- Build images on GitHub runners
- Tidy up GHA workflows
- Set version on root command
- Enable Homebrew cask generation
- Temporarily disable notarization
- Re-enable module proxying

## [0.1.7-alpha.2] - 2026-04-21

### Other

- Fix release attestation
- Fix macOS notarization

## [0.1.7-alpha.1] - 2026-04-21

### Other

- Update repo references from xenforo-ltd/xf to xenforo-ltd/cli
- Create dependabot.yml
- Update a few other references to reflect new repo name
- Refactor Download and DownloadWithAuth into shared helper
- Fix checksum validation ordering to run before file rename
- Use string functions from the standard lib
- Attempt to pull MySQL credentials from env file
- Fix formatting
- Code clean up
- Fix formatting
- Tidy dependencies
- Bump dependencies
- Fix Makefile phony targets
- Adjust root command long description
- Fix explicit embedded struct usages
- Fix double allocation when building strings
- Remove deprecated `tar.TypeRegA` check
- Return a struct for OAuth endpoints
- Reorder methods to be public first
- Fix unnecessary nested condition
- Update comment formatting
- Use integer ranges
- Use `maps.Copy` where appropriate instead of loops
- Iterate over strings using `strings.SplitSeq`
- Use `slices.Contains` to check command aliases
- Use `strings.CutPrefix` over `HasPrefix` and `TrimPrefix`
- Use `max` function where appropriate
- Use `strings.Cut` where appropriate
- Use string builder instead of concatenating in a loop
- "omitempty"` has no effect on embedded structs
- Minor nits
- Improve init flows
- Resolve cache conflict
- Fix formatting
- Minor nits
- Always close response bodies
- Rename base module
- Rename `errors` package to `clierrors`
- Prefer `errors.Is` over direct comparison
- Minor nits
- Replace dynamic errors with wrapped static errors
- Use `errors.AsType` to unwrap errors
- Improve documentation
- Address stuttering
- Move some magic strings to constants
- Do not swallow errors when checking the cache
- Use explicit error to signal metadata was not found
- Use context-aware APIs
- Use built-in `t.Chdir` function over bespoke helper
- Use built-in `t.TempDir` function to create test temporary directories
- Make use of `sync.WaitGroup.Go`
- More opinionated formatting
- Minor nits
- Rename `api` package to `customerapi`
- Rename `embed` package to `docker`
- Minor adjustments to Docker configuration
- Minor nits
- Cut down on code duplication
- Remove dead code
- Adjust documentation
- Fix user agent string
- Split compose command into its own file
- Use the proper OS cache directory instead of caching into the config directory
- Use built-in user config directory function instead of reimplementing it poorly
- Fix CI image publishing workflow
- Replace bespoke configuration management with Viper
- Update Lipgloss to v2
- Bump deps
- Use `v0.0.0` as development version
- Use Go version from go.mod instead of hard-coding it several times
- Fix formatting
- Bump the github-actions group across 1 directory with 9 updates
- Fix Docker CI/CD paths
- Bump the gomod group with 2 updates
- Clean up
- Fix context usage
- Handle errors
- Remove duplicate test
- Clean up
- Use string concatenation where possible
- Update Docker dependencies
- Update huh to v2 and update dependencies
- Remove dead code
- Clean up some constants
- Tighten up file permissions
- Set sensible timeouts on the OAuth callback server
- Ensure decompression file sizes are reasonable
- Harden `OpenBrowser` function
- Harden against path traversal
- Harden free disk space command
- Clean up some magic numbers
- Rework error handling
- Code clean up
- Improve test speeds a bit
- Move CLI to subdirectory and improve Makefile
- Fix unclosed file handle during extraction
- Code clean-up
- Update documentation and meta files
- Update GitHub meta files
- Add VSCode configuration
- Update GHA workflows
- Improve CI/CD workflows
- Fix Makefile
- Bump charm.land/lipgloss/v2 from 2.0.2 to 2.0.3 in the gomod group
- Temporarily disable module proxying

## [0.1.6] - 2026-02-20

### Other

- Release v0.1.6

## [0.1.5] - 2026-02-20

### Other

- Release v0.1.5

## [0.1.4] - 2026-02-20

### Other

- Handle legacy PKCS#12 ciphers in macOS signing workflow
- Release v0.1.4

## [0.1.3] - 2026-02-20

### Other

- Release v0.1.3
- Fix macOS p12 decode/import in release workflow
- Harden macOS signing/notary secret decoding in release workflow

## [0.1.2] - 2026-02-20

### Other

- Harden init directory guardrails and release v0.1.2

## [0.1.1] - 2026-02-20

### Other

- Add local fallback for XenForo commands when Docker config missing
- Remove extra blank line in root.go

## [0.1.0] - 2026-02-20

### Other

- Initial commit
- Do not generate provenance attestations
- Automatically remove old CI images
- Bump actions/checkout from 3 to 4
- Bump docker/login-action from 2 to 3
- Bump docker/metadata-action from 4 to 5
- Bump docker/build-push-action from 4 to 5
- Bump docker/setup-buildx-action from 2 to 3
- Bump actions/delete-package-versions from 4 to 5
- Build docker with intl extension
- Bump docker/build-push-action from 5 to 6
- Revise packaged PHP versions
- Generate PHP 7.4 image
- Replace 7.4 with 8.0
- Bump Composer to v2.8
- Update copyright year
- Add `.editorconfig` file
- Move files into instance directory
- Order base PHP extension installations alphabetically
- Remove `extra_hosts` for XF service
- Remove Adminer
- Add service dependencies
- Add network partitioning
- Rename volumes to be less verbose
- Add cache volume
- Configure Xdebug with environment variables
- Remove XDebug support for PHP <8
- Update configuration
- Auto-configure build and environment from contexts
- Bump dependencies
- Improve default PHP performance settings
- Improve default database performance settings
- Bump max upload size to `128M`
- Fix an issue with installing the PostgreSQL extension
- Update config for XF 2.3
- Bundle GD AVIF support on PHP 8.1+
- Install PDO extensions for database drivers
- Introduce `replication` context to automatically enable database replication
- Introduce `mssql` context for Microsoft SQL Server driver support
- Clean up Dockerfile
- Clean up management utility
- Default `ps` command to comprehensive service information
- Replace `restart` command with `reboot` command
- Replace `build` and `pull` commands with `update` command
- Remove `kill` command
- Change Compose project name and context behavior
- Use ephemeral port mapping and auto-set an Orbstack domain
- Allow customizing default `.dockerignore` and `.env` files
- Rename services and make MySQL service optional
- Add descriptions to commands
- Fix GHA (hopefully)
- Fix basic PHP 7.2 support
- Retain MySQL support for CI images
- Bump Docker image `latest` tag to 8.4
- Introduce `mailpit` context to autoconfigure Mailpit
- Remove hard-coded mailpit credentials
- Bump dependencies
- Build PHP 8.0 CI image
- Fix XDebug mode support
- Fix an issue parsing contexts with hyphens
- Make nginx optional
- Set most environment variable defaults via Compose
- Support Caddy as an alternative to nginx
- Bump dependencies
- Allow overriding `PHP_BUILD_*` environment variables in `.env`
- Bump dependencies
- Bump actions/checkout from 4 to 5
- Bump Docker dependencies
- Bump dependencies
- Bump dependencies
- Bump actions/checkout from 5 to 6
- Bump dependencies
- Adjust Dependabot interval to monthly
- Group Dependabot updates
- Bump dependencies
- Fix building PHP 8.5
- Bump Xdebug version
- Bump default PHP version to 8.5
- Bump dependencies
- Clean up
- Drop nginx in favor of Caddy
- Include zip and unzip tools in image
- Introduce XF CLI tool

[0.2.0]: https://github.com/xenforo-ltd/cli/compare/v0.1.7-alpha.7..v0.2.0
[0.1.7-alpha.7]: https://github.com/xenforo-ltd/cli/compare/v0.1.7-alpha.6..v0.1.7-alpha.7
[0.1.7-alpha.6]: https://github.com/xenforo-ltd/cli/compare/v0.1.7-alpha.5..v0.1.7-alpha.6
[0.1.7-alpha.5]: https://github.com/xenforo-ltd/cli/compare/v0.1.7-alpha.4..v0.1.7-alpha.5
[0.1.7-alpha.4]: https://github.com/xenforo-ltd/cli/compare/v0.1.7-alpha.3..v0.1.7-alpha.4
[0.1.7-alpha.3]: https://github.com/xenforo-ltd/cli/compare/v0.1.7-alpha.2..v0.1.7-alpha.3
[0.1.7-alpha.2]: https://github.com/xenforo-ltd/cli/compare/v0.1.7-alpha.1..v0.1.7-alpha.2
[0.1.7-alpha.1]: https://github.com/xenforo-ltd/cli/compare/v0.1.6..v0.1.7-alpha.1
[0.1.6]: https://github.com/xenforo-ltd/cli/compare/v0.1.5..v0.1.6
[0.1.5]: https://github.com/xenforo-ltd/cli/compare/v0.1.4..v0.1.5
[0.1.4]: https://github.com/xenforo-ltd/cli/compare/v0.1.3..v0.1.4
[0.1.3]: https://github.com/xenforo-ltd/cli/compare/v0.1.2..v0.1.3
[0.1.2]: https://github.com/xenforo-ltd/cli/compare/v0.1.1..v0.1.2
[0.1.1]: https://github.com/xenforo-ltd/cli/compare/v0.1.0..v0.1.1
[0.1.0]: https://github.com/xenforo-ltd/cli/tree/v0.1.0
