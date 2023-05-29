# XenForo Docker Environment

[![Publish CI images](https://github.com/xenforo-ltd/docker/actions/workflows/publish-ci.yml/badge.svg)](https://github.com/xenforo-ltd/docker/actions/workflows/publish-ci.yml)

The environment includes Nginx, PHP with all required extensions and additional
optional extensions and tools, and MariaDB. It includes optional support for
Redis caching, Elasticsearch for the XenForo Enhanced Search add-on, and
PostgreSQL for Discourse imports.

## Requirements

- Bash
- Docker
- Docker Compose

## Installing

1. Clone this repository
2. Add the `bin/` directory to your shell path
3. Run `xf init` from the XenForo installation
4. Edit the `.env` file in the XenForo installation to configure the environment
5. Run `xf up` to start the environment
6. Run `xf xf:install` to install XenForo

## Updating

1. Pull changes from this repository
3. Run `xf init` from the XenForo installation

## Configuring

Many common options may be set in the `.env` file.  You may create a
`src/config.override.php` file in the XenForo installation to further customize
the XenForo configuration. You may create a `compose.override.yaml` file in the
XenForo installation to customize the Docker Compose configuration.
