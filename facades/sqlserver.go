package facades

import (
	"github.com/gradii/framework/contracts/database/driver"

	"github.com/gradii/sqlserver"
	drivererrors "github.com/gradii/sqlserver/errors"
)

func Sqlserver(connection string) (driver.Driver, error) {
	if sqlserver.App == nil {
		return nil, drivererrors.ServiceProviderNotRegistered
	}

	instance, err := sqlserver.App.MakeWith(sqlserver.Binding, map[string]any{
		"connection": connection,
	})
	if err != nil {
		return nil, err
	}

	return instance.(*sqlserver.Sqlserver), nil
}
