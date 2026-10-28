package sqlserver

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"os"
	"testing"
	"time"

	frameworkconfig "github.com/gradii/framework/config"
	"github.com/gradii/framework/contracts/database"
	contractsdriver "github.com/gradii/framework/contracts/database/driver"
	contractsschema "github.com/gradii/framework/contracts/database/schema"
	frameworkdb "github.com/gradii/framework/database/db"
	frameworkorm "github.com/gradii/framework/database/gorm"
	frameworkschema "github.com/gradii/framework/database/schema"
	"github.com/gradii/framework/testing/utils"
	"github.com/microsoft/go-mssqldb/msdsn"
	"github.com/stretchr/testify/require"
)

type integrationRecord struct {
	StableKey      uint64 `gorm:"column:id;primaryKey;autoIncrement" db:"id"`
	StableLabel    string `gorm:"column:display_name" db:"display_name"`
	StableDocument string `gorm:"column:document" db:"document"`
}

func (integrationRecord) TableName() string { return "driver_users" }

func TestFrameworkIntegration(t *testing.T) {
	connectionString := os.Getenv("GRADII_TEST_SQLSERVER_DSN")
	if connectionString == "" {
		t.Skip("set GRADII_TEST_SQLSERVER_DSN to use an existing SQL Server test database")
	}
	if os.Getenv("GRADII_TEST_SQLSERVER_CREATE_DATABASE") == "1" {
		connector, err := NewConnector(database.Config{Dsn: connectionString})
		require.NoError(t, err)
		admin := sql.OpenDB(connector)
		isolatedDatabase := fmt.Sprintf("gdi_sqlserver_%x", time.Now().UnixNano())
		_, err = admin.Exec("create database [" + isolatedDatabase + "]")
		require.NoError(t, err)
		t.Cleanup(func() {
			_, err := admin.Exec("alter database [" + isolatedDatabase + "] set single_user with rollback immediate; drop database [" + isolatedDatabase + "]")
			require.NoError(t, err)
			require.NoError(t, admin.Close())
		})
		address, err := url.Parse(connectionString)
		require.NoError(t, err)
		parameters := address.Query()
		parameters.Set("database", isolatedDatabase)
		address.RawQuery = parameters.Encode()
		connectionString = address.String()
	}
	parsed, err := msdsn.Parse(connectionString)
	require.NoError(t, err)
	name := fmt.Sprintf("sqlserver_driver_%d", time.Now().UnixNano())
	prefix := fmt.Sprintf("gdi_%x_", time.Now().UnixNano())
	config := frameworkconfig.NewApplication("")
	config.Add("database.default", name)
	config.Add("database.pool", map[string]any{"max_open_conns": 1, "max_idle_conns": 1})
	config.Add("database.connections."+name, map[string]any{"driver": "sqlserver", "dsn": connectionString, "database": parsed.Database, "prefix": prefix, "timezone": "UTC", "via": func() (contractsdriver.Driver, error) { return NewSqlserver(config, nil, nil, name), nil }})
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	log := utils.NewTestLog()
	db, err := frameworkdb.BuildDB(ctx, config, log, name, nil)
	require.NoError(t, err)
	t.Cleanup(func() {
		cleanup, stop := context.WithTimeout(context.Background(), 10*time.Second)
		defer stop()
		for _, table := range []string{"driver_children", "renamed_users", "driver_users"} {
			require.NoError(t, db.StatementContext(cleanup, "if object_id('["+prefix+table+"]', 'U') is not null drop table ["+prefix+table+"]"))
		}
		require.NoError(t, db.Purge())
	})
	driver := NewSqlserver(config, nil, nil, name)
	schema, err := frameworkschema.NewSchema(config, log, nil, driver, nil, db)
	require.NoError(t, err)
	require.NoError(t, schema.Create("driver_users", func(table contractsschema.Blueprint) {
		table.BigIncrements("id")
		table.String("display_name", 100).Comment("user label")
		table.Json("document").Nullable()
		table.DateTime("occurred_at", 6).Nullable()
		table.Unique("display_name").Name(prefix + "name_unique")
		table.Comment("user records")
	}))
	require.NoError(t, schema.Create("driver_children", func(table contractsschema.Blueprint) {
		table.BigIncrements("id")
		table.BigInteger("user_id")
		table.Foreign("user_id").References("id").On("driver_users").CascadeOnDelete().Name(prefix + "child_foreign")
	}))

	t.Run("DB output identity and JSON update", func(t *testing.T) {
		id, err := db.Table("driver_users").InsertGetID(map[string]any{"display_name": "DB 你好", "document": `{"roles":["reader"]}`})
		require.NoError(t, err)
		require.Positive(t, id)
		_, err = db.Table("driver_users").Where("id", id).Update(map[string]any{"document->roles": []any{"writer", "admin"}, "document->enabled": true})
		require.NoError(t, err)
		count, err := db.Table("driver_users").WhereJsonContains("document->roles", "admin").Count()
		require.NoError(t, err)
		require.EqualValues(t, 1, count)
		var records []integrationRecord
		require.NoError(t, db.Table("driver_users").Select("id", "display_name", "document").Get(&records))
		require.Len(t, records, 1)
		require.JSONEq(t, `{"roles":["writer","admin"],"enabled":true}`, records[0].StableDocument)
	})
	query, _, err := frameworkorm.BuildQuery(ctx, config, name, log, nil, nil)
	require.NoError(t, err)
	t.Run("ORM explicit mapping and transaction output", func(t *testing.T) {
		record := integrationRecord{StableLabel: "ORM 你好", StableDocument: `{"roles":[]}`}
		require.NoError(t, query.Clone().Create(&record))
		require.Positive(t, record.StableKey)
		var loaded integrationRecord
		require.NoError(t, query.Clone().Where("id", record.StableKey).First(&loaded))
		require.Equal(t, record.StableLabel, loaded.StableLabel)
		tx, err := query.Clone().BeginTransaction()
		require.NoError(t, err)
		defer tx.Rollback()
		transient := integrationRecord{StableLabel: "rolled back", StableDocument: `{}`}
		require.NoError(t, tx.Create(&transient))
		require.Positive(t, transient.StableKey)
		require.NoError(t, tx.Rollback())
		count, err := query.Clone().Model(&integrationRecord{}).Where("id", transient.StableKey).Count()
		require.NoError(t, err)
		require.Zero(t, count)
	})
	t.Run("schema alteration metadata and comments", func(t *testing.T) {
		require.NoError(t, schema.Table("driver_users", func(table contractsschema.Blueprint) {
			table.String("notes", 40).Nullable().Default("initial").Comment("notes comment")
			table.Computed("name_length", "len([display_name])").Persisted()
		}))
		columns, err := schema.GetColumns("driver_users")
		require.NoError(t, err)
		byName := map[string]contractsdriver.Column{}
		for _, column := range columns {
			byName[column.Name] = column
		}
		require.Equal(t, "nvarchar(100)", byName["display_name"].Type)
		require.Equal(t, "datetime2(6)", byName["occurred_at"].Type)
		require.Equal(t, "user label", byName["display_name"].Comment)
		require.Equal(t, "notes comment", byName["notes"].Comment)
		require.NotNil(t, byName["name_length"].Generation)
		indexes, err := schema.GetIndexes("driver_users")
		require.NoError(t, err)
		require.Len(t, indexes, 2)
		keys, err := schema.GetForeignKeys("driver_children")
		require.NoError(t, err)
		require.Len(t, keys, 1)
		require.Equal(t, []string{"user_id"}, keys[0].Columns)
		tables, err := schema.GetTables()
		require.NoError(t, err)
		found := false
		for _, table := range tables {
			if table.Name == prefix+"driver_users" {
				found = true
				require.Equal(t, "user records", table.Comment)
			}
		}
		require.True(t, found)
		require.NoError(t, schema.Table("driver_users", func(table contractsschema.Blueprint) {
			table.String("notes", 80).Nullable().Default("changed").Comment("changed comment").Change()
		}))
		require.NoError(t, schema.Table("driver_users", func(table contractsschema.Blueprint) { table.RenameColumn("notes", "remarks") }))
		require.True(t, schema.HasColumn("driver_users", "remarks"))
		require.NoError(t, schema.Table("driver_users", func(table contractsschema.Blueprint) { table.DropColumn("remarks") }))
		require.False(t, schema.HasColumn("driver_users", "remarks"))
		require.NoError(t, schema.Rename("driver_users", "renamed_users"))
		require.True(t, schema.HasTable("renamed_users"))
	})
}
