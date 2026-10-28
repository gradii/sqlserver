package sqlserver

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	frameworkconfig "github.com/gradii/framework/config"
	contractsdb "github.com/gradii/framework/contracts/database/db"
	contractsdriver "github.com/gradii/framework/contracts/database/driver"
	contractsschema "github.com/gradii/framework/contracts/database/schema"
	frameworkdb "github.com/gradii/framework/database/db"
	frameworkschema "github.com/gradii/framework/database/schema"
	"github.com/gradii/framework/errors"
	"github.com/gradii/framework/testing/utils"
	"github.com/stretchr/testify/require"
)

func ddlIntegration(t *testing.T) (contractsschema.Schema, contractsdb.DB, string) {
	t.Helper()
	dsn := os.Getenv("GRADII_TEST_SQLSERVER_DSN")
	if dsn == "" {
		t.Skip("set GRADII_TEST_SQLSERVER_DSN to exercise native schema changes")
	}
	name := fmt.Sprintf("ddl_sqlserver_%x", time.Now().UnixNano())
	namespace := fmt.Sprintf("ddl_%x", time.Now().UnixNano())
	config := frameworkconfig.NewApplication("")
	config.Add("database.default", name)
	config.Add("database.pool", map[string]any{"max_open_conns": 1, "max_idle_conns": 1})
	config.Add("database.connections."+name, map[string]any{
		"driver": "sqlserver", "dsn": dsn,
		"via": func() (contractsdriver.Driver, error) { return NewSqlserver(config, nil, nil, name), nil },
	})
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	t.Cleanup(cancel)
	log := utils.NewTestLog()
	db, err := frameworkdb.BuildDB(ctx, config, log, name, nil)
	require.NoError(t, err)
	require.NoError(t, db.Statement("create schema "+namespace))
	t.Cleanup(func() {
		require.NoError(t, db.StatementContext(context.Background(), "drop schema "+namespace+""))
		require.NoError(t, db.Purge())
	})
	s, err := frameworkschema.NewSchema(config, log, nil, NewSqlserver(config, nil, nil, name), nil, db)
	require.NoError(t, err)
	return s, db, namespace
}

func TestSchemaDDLIntegration(t *testing.T) {
	s, db, namespace := ddlIntegration(t)
	table := func(name string) string { return namespace + "." + name }
	cleanup := func(t *testing.T, name string) {
		t.Helper()
		t.Cleanup(func() { require.NoError(t, s.DropIfExists(table(name))) })
	}
	t.Run("enum change preserves other checks and rolls back invalid restrictions", func(t *testing.T) {
		name := table("enum_items")
		cleanup(t, "enum_items")
		require.NoError(t, s.Create(name, func(b contractsschema.Blueprint) {
			b.Integer("id")
			b.Enum("state", []any{"open", "closed", ""}).Nullable().Default("open").Comment("old state")
			b.String("note", 30).Nullable()
			b.Index("state").Name("enum_state_idx")
		}))
		require.NoError(t, db.Statement("alter table "+name+" add constraint extra_length check (len(state) < 20)"))
		require.NoError(t, db.Statement("insert into "+name+" (id, state) values (?, ?)", 1, "open"))
		require.NoError(t, s.Table(name, func(b contractsschema.Blueprint) {
			b.Enum("state", []any{"open", "closed", "", "new", "O'Reilly", "中文"}).Nullable().Default("new").Comment("new state").Change()
			b.String("note", 50).Nullable().Change()
		}))
		for i, value := range []string{"new", "O'Reilly", "中文", ""} {
			require.NoError(t, db.Statement("insert into "+name+" (id, state) values (?, ?)", i+2, value))
		}
		require.NoError(t, db.Statement("insert into "+name+" (id) values (?)", 20))
		var rows []map[string]any
		require.NoError(t, db.Select(&rows, "select state from "+name+" where id = 20"))
		require.Equal(t, "new", fmt.Sprint(rows[0]["state"]))
		require.Error(t, db.Statement("insert into "+name+" (id, state) values (?, ?)", 90, "invalid"))
		require.Error(t, s.Table(name, func(b contractsschema.Blueprint) {
			b.Enum("state", []any{"closed"}).Nullable().Default("closed").Change()
			b.String("note", 80).Nullable().Change()
		}))
		// A failed restriction restores the previous membership and default.
		require.NoError(t, db.Statement("insert into "+name+" (id, state) values (?, ?)", 91, "new"))
		require.NoError(t, db.Statement("insert into "+name+" (id) values (?)", 92))
		rows = nil
		require.NoError(t, db.Select(&rows, "select state from "+name+" where id = 92"))
		require.Equal(t, "new", fmt.Sprint(rows[0]["state"]))
		indexes, err := s.GetIndexes(name)
		require.NoError(t, err)
		require.Len(t, indexes, 1)
		require.Equal(t, "enum_state_idx", indexes[0].Name)
		// Expand to a value disallowed by the independent CHECK; it must survive.
		longValue := "this_label_is_longer_than_twenty"
		require.NoError(t, s.Table(name, func(b contractsschema.Blueprint) {
			b.Enum("state", []any{"open", "closed", "", "new", "O'Reilly", "中文", longValue}).Nullable().Change()
		}))
		require.Error(t, db.Statement("insert into "+name+" (id, state) values (?, ?)", 93, longValue))
	})
	t.Run("legacy enum check and custom membership boundary", func(t *testing.T) {
		name := table("legacy_items")
		cleanup(t, "legacy_items")
		require.NoError(t, db.Statement("create table "+name+" (state nvarchar(255) check (state in ('a', 'b')) not null)"))
		require.NoError(t, db.Statement("alter table "+name+" add check (len(state) < 20)"))
		require.NoError(t, s.Table(name, func(b contractsschema.Blueprint) { b.Enum("state", []any{"a", "b", "c"}).Change() }))
		require.NoError(t, db.Statement("insert into "+name+" (state) values (?)", "c"))

		custom := table("custom_items")
		cleanup(t, "custom_items")
		require.NoError(t, db.Statement("create table "+custom+" (state nvarchar(255) not null, constraint custom_membership check (state in ('a', 'b')))"))
		err := s.Table(custom, func(b contractsschema.Blueprint) { b.Enum("state", []any{"a", "b", "c"}).Change() })
		require.ErrorIs(t, err, errors.SchemaEnumConstraintAmbiguous)
		require.NoError(t, db.Statement("insert into "+custom+" (state) values (?)", "a"))
		require.Error(t, db.Statement("insert into "+custom+" (state) values (?)", "c"))
	})
	t.Run("current defaults and Unicode literals survive changes", func(t *testing.T) {
		name := table("default_items")
		cleanup(t, "default_items")
		require.NoError(t, s.Create(name, func(b contractsschema.Blueprint) {
			b.Integer("id")
			b.Timestamp("created_at", 4)
			b.String("label", 80).Default("中文🌏 O'Reilly")
		}))
		for i := 0; i < 2; i++ {
			require.NoError(t, s.Table(name, func(b contractsschema.Blueprint) { b.Timestamp("created_at", 4).UseCurrent().Change() }))
			require.NoError(t, db.Statement("insert into "+name+" (id) values (?)", i))
		}
		var rows []map[string]any
		require.NoError(t, db.Select(&rows, "select label, created_at from "+name))
		require.Len(t, rows, 2)
		for _, row := range rows {
			require.Equal(t, "中文🌏 O'Reilly", fmt.Sprint(row["label"]))
			require.NotNil(t, row["created_at"])
		}
		require.NoError(t, s.Table(name, func(b contractsschema.Blueprint) {
			b.String("label", 100).Default("変更🌍 'quoted'").Change()
			b.String("added", 100).Default("追加🌍")
		}))
		require.NoError(t, db.Statement("insert into "+name+" (id) values (?)", 3))
		rows = nil
		require.NoError(t, db.Select(&rows, "select label, added from "+name+" where id = 3"))
		require.Equal(t, "変更🌍 'quoted'", fmt.Sprint(rows[0]["label"]))
		require.Equal(t, "追加🌍", fmt.Sprint(rows[0]["added"]))
	})
	t.Run("persisted computed explicit not null is enforced", func(t *testing.T) {
		name := table("computed_items")
		cleanup(t, "computed_items")
		require.NoError(t, s.Create(name, func(b contractsschema.Blueprint) {
			b.Integer("source").Nullable()
			b.Computed("required_value", "[source] + 1").Persisted().Nullable(false)
			b.Computed("optional_value", "[source] + 2").Persisted().Nullable()
		}))
		require.NoError(t, db.Statement("insert into "+name+" (source) values (?)", 3))
		require.Error(t, db.Statement("insert into "+name+" (source) values (null)"))
		columns, err := s.GetColumnsMetadata(name)
		require.NoError(t, err)
		for _, column := range columns {
			if column.Name == "required_value" {
				require.False(t, column.Nullable)
			}
		}
		invalid := table("invalid_computed")
		err = s.Create(invalid, func(b contractsschema.Blueprint) { b.Integer("source"); b.Computed("v", "source + 1").Nullable(false) })
		require.ErrorIs(t, err, errors.SchemaComputedNotNullRequiresPersisted)
		require.False(t, s.HasTable(invalid))
	})
}
