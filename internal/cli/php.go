package cli

import (
	"github.com/spf13/cobra"

	"github.com/xenforo-ltd/cli/internal/commands/php"
	"github.com/xenforo-ltd/cli/internal/commands/phpdebug"
)

// newPHPCommand builds the php passthrough command. Arguments and flags after
// its name belong to PHP; xf flags must precede it.
func newPHPCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "php [path] [args...]",
		Short: "Run PHP commands",
		Long: `Run PHP in the Docker environment.

If no path is provided, the current directory will be searched for a XenForo installation.

Everything after 'php' is passed to PHP, including flags. Give xf's own flags
before the command name, and use 'xf help php' for this help text.`,
		Example: `  # Check PHP version
  xf php -v

  # Run a PHP script
  xf php my-script.php

  # Run PHP in specific directory
  xf php ./my-project -v

  # Enable xf verbose output (before the command name)
  xf --verbose php my-script.php`,
		DisableFlagParsing: true,
		Args:               cobra.ArbitraryArgs,
		GroupID:            "run",
		RunE: func(cmd *cobra.Command, args []string) error {
			return php.Run(cmd.Context(), args)
		},
	}
}

// newPHPDebugCommand builds the php-debug passthrough command. Arguments and
// flags after its name belong to PHP; xf flags must precede it.
func newPHPDebugCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "php-debug [path] [args...]",
		Short: "Run PHP with Xdebug",
		Long: `Run PHP with Xdebug enabled in the Docker environment.

If no path is provided, the current directory will be searched for a XenForo installation.
All arguments are passed to PHP.`,
		Example: `  # Run PHP script with Xdebug
  xf php-debug my-script.php`,
		DisableFlagParsing: true,
		Args:               cobra.ArbitraryArgs,
		GroupID:            "run",
		RunE: func(cmd *cobra.Command, args []string) error {
			return phpdebug.Run(cmd.Context(), args)
		},
	}
}
