// Package database describes the database a XenForo environment is configured
// to use, and how to reach it from a client running on the host.
package database

import (
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"
)

// Driver identifies a database engine supported by the environment.
type Driver string

const (
	DriverMySQL    Driver = "mysql"
	DriverPostgres Driver = "postgres"
	DriverMSSQL    Driver = "mssql"
	DriverSQLite   Driver = "sqlite"
)

// DefaultPort returns the TCP port the driver listens on, or 0 for file-based
// drivers such as SQLite.
func (d Driver) DefaultPort() int {
	switch d {
	case DriverMySQL:
		return 3306
	case DriverPostgres:
		return 5432
	case DriverMSSQL:
		return 1433
	default:
		return 0
	}
}

// Scheme returns the URL scheme clients expect for the driver.
func (d Driver) Scheme() string {
	switch d {
	case DriverMySQL:
		return "mysql"
	case DriverPostgres:
		return "postgresql"
	case DriverMSSQL:
		return "mssql"
	case DriverSQLite:
		return "sqlite"
	default:
		return ""
	}
}

// Detect resolves the driver selected by a set of compose contexts, as found
// in XF_CONTEXTS.
//
// The first database context wins. The database engine is not declared
// anywhere else the host can read, so the context is the source of truth until
// XenForo itself can be asked.
func Detect(contexts []string) (Driver, error) {
	for _, ctx := range contexts {
		switch strings.TrimSpace(ctx) {
		case string(DriverMySQL), "mysql-replication":
			return DriverMySQL, nil
		case string(DriverPostgres):
			return DriverPostgres, nil
		case string(DriverMSSQL):
			return DriverMSSQL, nil
		case string(DriverSQLite):
			return DriverSQLite, nil
		}
	}

	return "", fmt.Errorf("no supported database context in %q", strings.Join(contexts, ":"))
}

// Info describes a resolved database connection.
type Info struct {
	Driver   Driver
	Host     string
	Port     int
	User     string
	Password string
	Name     string
	FilePath string
}

// URL renders the connection URL used to open the database in a GUI or CLI
// client. SQLite is file-based, so its "URL" is the file path.
func (i Info) URL() string {
	if i.Driver == DriverSQLite {
		return i.FilePath
	}

	u := &url.URL{
		Scheme: i.Driver.Scheme(),
		User:   url.UserPassword(i.User, i.Password),
		Host:   net.JoinHostPort(i.Host, strconv.Itoa(i.Port)),
		Path:   "/" + i.Name,
	}

	return u.String()
}

// OrbStackHost returns the hostname OrbStack gives a compose service, which
// resolves from macOS without publishing ports: <service>.<project>.orb.local.
func OrbStackHost(service, instance string) string {
	return service + "." + instance + ".orb.local"
}
