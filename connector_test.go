package sqlserver

import (
	"testing"
	"time"

	"github.com/gradii/framework/contracts/database"
	drivererrors "github.com/gradii/sqlserver/errors"
	"github.com/microsoft/go-mssqldb/msdsn"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConnectorDSNPreservesValuesAndNativeOptions(t *testing.T) {
	endpoint := database.Config{Host: "::1", Port: 1433, Database: "app db", Username: "user:@x", Password: "p/@?#%word", Charset: "utf8", Timezone: "Asia/Shanghai"}
	connectionString, err := connectorDSN(endpoint)
	require.NoError(t, err)
	parsed, err := msdsn.Parse(connectionString)
	require.NoError(t, err)
	assert.Equal(t, endpoint.Username, parsed.User)
	assert.Equal(t, endpoint.Password, parsed.Password)
	assert.Equal(t, endpoint.Database, parsed.Database)
	assert.Equal(t, "Asia/Shanghai", parsed.Encoding.GetTimezone().String())
	assert.Equal(t, "true", parsed.Parameters["multipleactiveresultsets"])
	assert.Equal(t, "utf8", parsed.Parameters["charset"])
	connector, err := NewConnector(endpoint)
	require.NoError(t, err)
	require.NotNil(t, connector)
	explicit := "sqlserver://custom:secret@127.0.0.1:1433?database=other&encrypt=disable&timezone=UTC&MultipleActiveResultSets=false"
	endpoint.Dsn = explicit
	connectionString, err = connectorDSN(endpoint)
	require.NoError(t, err)
	assert.Equal(t, explicit, connectionString)
	parsed, err = msdsn.Parse(connectionString)
	require.NoError(t, err)
	assert.Equal(t, time.UTC, parsed.Encoding.GetTimezone())
	assert.Equal(t, "false", parsed.Parameters["multipleactiveresultsets"])
}
func TestConnectorRejectsInvalidSettingsWithoutConnecting(t *testing.T) {
	_, err := NewConnector(database.Config{Host: "localhost", Port: 70000})
	assert.ErrorIs(t, err, drivererrors.InvalidPort)
	_, err = NewConnector(database.Config{Host: "localhost", Timezone: "missing/timezone"})
	require.Error(t, err)
	_, err = NewConnector(database.Config{Dsn: "sqlserver://localhost?timezone=missing/timezone"})
	require.Error(t, err)
}
