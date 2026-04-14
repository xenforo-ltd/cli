# Contributing

Thank you for your interest in contributing to the XenForo CLI.

We accept pull requests for any bugs, as well as feature requests labeled `help
wanted`. We encourage new issues for all other contributions.

## Notes

- Before opening an issue, check that one does not already exist for the same problem or feature.
- Open a bug report if things are not working as expected.
- Open a feature request to propose a change.
- Open a pull request for any bug as well as feature requests labeled [`help wanted`][help wanted].
- Do not an issue for support. Use the [development discussions forum][forum] instead.
- Do not open a pull request for issues without the `help wanted` label.
- Do not pull request scope to include changes that are not described in the corresponding issue.

## Development

### Building

1. Install Go.
2. Clone the repository.
3. Run `make` to build the binary.
4. Run `dist/xf` to use the built binary.

### Tips

- Use `make fmt` to format the code.
- Use `make vet` to lint the code.
- Use `make test` to test the code.

Contributions to this project are released to the public under the [project's
open source license][license].

Please note that this project adheres to a [Contributor Code of
Conduct][code-of-conduct]. By participating in this project you agree to abide
by its terms.

[help wanted]: https://github.com/xenforo/cli/labels/help%20wanted
[forum]: https://xenforo.com/community/forums/xenforo-development-discussions.34/
[license]: ./LICENSE
[code-of-conduct]: ./CODE_OF_CONDUCT.md
