package sqlserver

import (
	"github.com/gradii/framework/database/schema"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestSchemaCompilerKeepsOrdinaryAndAdvancedColumns(t *testing.T) {
	blueprint := schema.NewBlueprint(nil, "app_", "items")
	blueprint.Create()
	blueprint.ID().StartingValue(21)
	blueprint.String("name", 40).Collation("Latin1_General_100_CI_AS").Default("hello").Comment("column comment")
	blueprint.Binary("payload", 16).Fixed()
	blueprint.Computed("twice", "[id] * 2").Persisted()
	blueprint.Comment("table comment")
	statements, err := blueprint.CompileStatements(NewGrammar("app_"))
	require.NoError(t, err)
	require.NotEmpty(t, statements)
	assert.Contains(t, statements[0].SQL, "[id] bigint not null identity(21,1) primary key")
	assert.Contains(t, statements[0].SQL, "[name] nvarchar(40) collate Latin1_General_100_CI_AS not null default N'hello'")
	assert.Contains(t, statements[0].SQL, "[payload] binary(16) not null")
	assert.Contains(t, statements[0].SQL, "[twice] as ([id] * 2) persisted")
	comments := 0
	for _, statement := range statements {
		if len(statement.Bindings) == 4 {
			comments++
		}
	}
	assert.Equal(t, 2, comments)
}
func TestSchemaCompilerChangeRemovesDefaultsBeforeAlter(t *testing.T) {
	blueprint := schema.NewBlueprint(nil, "", "items")
	blueprint.String("name", 90).Nullable().Default("new").Change()
	statements, err := blueprint.CompileStatements(NewGrammar(""))
	require.NoError(t, err)
	require.Len(t, statements, 3)
	assert.Contains(t, statements[0].SQL, "quotename(object_name(c.default_object_id))")
	assert.Equal(t, []any{"[items]", "name"}, statements[0].Bindings)
	assert.Equal(t, "alter table [items] alter column [name] nvarchar(90) null", statements[1].SQL)
	assert.Equal(t, "alter table [items] add default N'new' for [name]", statements[2].SQL)
}
