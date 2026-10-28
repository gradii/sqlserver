package errors

import frameworkerrors "github.com/gradii/framework/errors"

var (
	DockerNotSupported           = frameworkerrors.New("Docker helpers are not supported by the SQL Server runtime driver")
	ServiceProviderNotRegistered = frameworkerrors.New("please register the SQL Server service provider")
	ConnectionNotSet             = frameworkerrors.New("SQL Server connection name is required")
	InvalidPort                  = frameworkerrors.New("invalid SQL Server port: %d")
	FailedToGenerateDSN          = frameworkerrors.New("failed to generate DSN, please check the database configuration")
	ConfigNotFound               = frameworkerrors.New("not found database configuration")
)
