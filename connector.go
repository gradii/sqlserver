package sqlserver

import (
	"database/sql/driver"
	"net"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/gradii/framework/contracts/database"
	"github.com/gradii/framework/errors"
	drivererrors "github.com/gradii/sqlserver/errors"
	native "github.com/microsoft/go-mssqldb"
)

// NewConnector validates one endpoint without opening a connection or owning a pool.
func NewConnector(endpoint database.Config) (driver.Connector, error) {
	if endpoint.Connector != nil {
		return endpoint.Connector, nil
	}
	connectionString, err := connectorDSN(endpoint)
	if err != nil {
		return nil, err
	}
	return native.NewConnector(connectionString)
}

func connectorDSN(endpoint database.Config) (string, error) {
	if endpoint.Dsn != "" {
		return endpoint.Dsn, nil
	}
	if endpoint.Host == "" {
		return "", errors.DatabaseConfigNotFound
	}
	port := endpoint.Port
	if port == 0 {
		port = 1433
	}
	if port < 1 || port > 65535 {
		return "", drivererrors.InvalidPort.Args(port)
	}
	timezone := endpoint.Timezone
	if timezone == "" {
		timezone = "UTC"
	}
	if _, err := time.LoadLocation(timezone); err != nil {
		return "", err
	}
	values := url.Values{"database": {endpoint.Database}, "charset": {endpoint.Charset}, "timezone": {timezone}, "MultipleActiveResultSets": {"true"}}
	connection := url.URL{Scheme: "sqlserver", User: url.UserPassword(endpoint.Username, endpoint.Password), Host: net.JoinHostPort(strings.Trim(endpoint.Host, "[]"), strconv.Itoa(port)), RawQuery: values.Encode()}
	return connection.String(), nil
}
