package db

import (
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"
)

// driver identifies a database engine supported by the environment.
type driver string

const (
	// driverMySQL identifies MySQL environments.
	driverMySQL driver = "mysql"
	// driverPostgres identifies PostgreSQL environments.
	driverPostgres driver = "postgres"
	// driverMSSQL identifies Microsoft SQL Server environments.
	driverMSSQL driver = "mssql"
	// driverSQLite identifies SQLite environments.
	driverSQLite driver = "sqlite"
)

// defaultPort returns the TCP port the driver listens on, or 0 for file-based
// drivers such as SQLite.
func (d driver) defaultPort() int {
	switch d {
	case driverMySQL:
		return 3306
	case driverPostgres:
		return 5432
	case driverMSSQL:
		return 1433
	default:
		return 0
	}
}

// scheme returns the URL scheme clients expect for the driver.
func (d driver) scheme() string {
	switch d {
	case driverMySQL:
		return "mysql"
	case driverPostgres:
		return "postgresql"
	case driverMSSQL:
		return "mssql"
	case driverSQLite:
		return "sqlite"
	default:
		return ""
	}
}

// detect resolves the driver selected by a set of compose contexts, as found
// in XF_CONTEXTS.
//
// The first database context wins. The database engine is not declared
// anywhere else the host can read, so the context is the source of truth until
// XenForo itself can be asked.
func detect(contexts []string) (driver, error) {
	for _, name := range contexts {
		switch strings.TrimSpace(name) {
		case string(driverMySQL), "mysql-replication":
			return driverMySQL, nil
		case string(driverPostgres):
			return driverPostgres, nil
		case string(driverMSSQL):
			return driverMSSQL, nil
		case string(driverSQLite):
			return driverSQLite, nil
		}
	}

	return "", fmt.Errorf("no supported database context in %q", strings.Join(contexts, ":"))
}

// connection describes a resolved database connection.
type connection struct {
	driver   driver
	host     string
	port     int
	user     string
	password string
	name     string
	filePath string
}

// urlString renders the connection URL used to open the database in a GUI or CLI
// client. SQLite is file-based, so its "URL" is the file path.
func (i connection) urlString() string {
	if i.driver == driverSQLite {
		return i.filePath
	}

	u := &url.URL{
		Scheme: i.driver.scheme(),
		User:   url.UserPassword(i.user, i.password),
		Host:   net.JoinHostPort(i.host, strconv.Itoa(i.port)),
		Path:   "/" + i.name,
	}

	return u.String()
}

// orbStackHost returns the hostname OrbStack gives a compose service, which
// resolves from macOS without publishing ports: <service>.<project>.orb.local.
func orbStackHost(service, instance string) string {
	return service + "." + instance + ".orb.local"
}
