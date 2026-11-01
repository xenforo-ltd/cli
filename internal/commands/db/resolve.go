package db

import (
	"fmt"

	"github.com/xenforo-ltd/cli/internal/clierror"
	"github.com/xenforo-ltd/cli/internal/environment"
	"github.com/xenforo-ltd/cli/internal/ui"
)

const databaseService = "mysql"

// resolveDatabaseInfo turns the environment's contexts and credentials into a
// connection description. Only MySQL is supported for now; the driver is
// detected so the unsupported case can say which engine it found.
func resolveDatabaseInfo(runner *environment.Runner) (connection, error) {
	driver, err := detect(runner.Contexts())
	if err != nil {
		return connection{}, clierror.WithHint(err, "Set XF_CONTEXTS to include a database, for example "+ui.Command.Render("mysql"))
	}

	if driver != driverMySQL {
		return connection{}, clierror.WithHint(
			fmt.Errorf("%s databases are not supported by %s yet", driver, ui.Command.Render("xf db")),
			"Only MySQL environments are supported for now",
		)
	}

	user, password := runner.DatabaseCredentials()

	return connection{
		Driver:   driver,
		Host:     orbStackHost(databaseService, runner.Instance()),
		Port:     driver.defaultPort(),
		User:     user,
		Password: password,
		Name:     runner.DatabaseName(),
	}, nil
}
