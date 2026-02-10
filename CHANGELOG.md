# Changelog

All notable changes to this project will be documented in this file.

## [Unreleased]

## [v0.1.1] - 2026-02-10

### Added
1. Unknown top-level commands now fall back to local XenForo execution (`php cmd.php <args...>`) when in a XenForo directory without Docker configuration (`compose.yaml` missing).

## [v1.0.0] - 2026-02-13

### Added
1. Cross-platform CI quality gates for formatting, vet, tests, and race tests.
2. Open-source project docs (`CONTRIBUTING`, `CODE_OF_CONDUCT`, `SECURITY`, issue/PR templates).

### Changed
1. Clarified `init` behavior: `XF_DEBUG` and `XF_DEVELOPMENT` are enforced for v1.
2. Hardened `self-update` checksum verification to fail closed when checksum retrieval fails.

### Fixed
1. Removed unreachable code in `cmd/init.go` causing `go vet` failure.
