package sqlserver

import (
	"strings"
	"testing"

	"github.com/gradii/framework/contracts/database/driver"
	"github.com/gradii/framework/database/schema"
	"github.com/gradii/framework/errors"
	"github.com/stretchr/testify/require"
)

type enumTestCatalog struct {
	rows    []map[string]any
	err     error
	invalid bool
}

func TestEmptyEnumValuesAreRejectedBeforeDDL(t *testing.T) {
	b := schema.NewBlueprint(nil, "", "items")
	b.Create()
	b.Enum("state", nil)
	statements, err := b.CompileStatements(NewGrammar(""))
	require.ErrorIs(t, err, errors.SchemaEnumValuesRequired)
	require.Empty(t, statements)
}

func (c enumTestCatalog) Select(statement driver.SchemaStatement) ([]map[string]any, error) {
	if strings.HasPrefix(statement.SQL, "select top 1") && c.err == nil {
		if c.invalid {
			return []map[string]any{{"invalid": 1}}, nil
		}
		return nil, nil
	}
	return c.rows, c.err
}
func (c enumTestCatalog) Columns(string) ([]driver.ColumnMetadata, error) { return nil, nil }
func (c enumTestCatalog) ResolveTable(string) (string, string, error)     { return "tenant", "items", nil }

func TestEnumMembershipRecognition(t *testing.T) {
	for _, test := range []struct {
		expression string
		valid      bool
	}{
		{"state in ('a', 'b')", true},
		{"([state]=N'a' OR [state]=N'b')", true},
		{"((state = 'a') OR (state = 'b'))", true},
		{"state in ('O''Reilly', '', '中文')", true},
		{"state in ('a') OR 1 = 1", false},
		{"lower(state) in ('a')", false},
		{"state in ('a') AND other = 'b'", false},
		{"state NOT IN ('a')", false},
		{"other in ('a')", false},
		{"state::integer = '1'", false},
		{"state::varchar(1) in ('a')", false},
		{"state::bpchar in ('a')", false},
		{"state in ('unterminated)", false},
	} {
		t.Run(test.expression, func(t *testing.T) { require.Equal(t, test.valid, enumMembership(test.expression, "state")) })
	}
}

func TestEnumChangePlansOwnedConstraints(t *testing.T) {
	g := NewGrammar("")
	b := schema.NewBlueprint(nil, "", "tenant.items")
	b.Enum("state", []any{"a", "b", "c"}).Nullable().Change()
	rows := []map[string]any{
		{"name": "CK__items__state", "definition": "state in ('a', 'b')", "single_column": true, "is_system_named": true},
		{"name": "custom_length", "definition": "length(state) < 20", "single_column": true, "is_system_named": true},
	}
	statements, handled, err := g.CompileSchemaBlueprint(b, enumTestCatalog{rows: rows})
	require.True(t, handled)
	require.NoError(t, err)
	require.Contains(t, statements[0].SQL, "drop constraint")
	require.Contains(t, statements[2].SQL, "nvarchar(255)")
	require.NotContains(t, statements[2].SQL, "check")
	require.Contains(t, statements[3].SQL, "add constraint")
	require.Contains(t, statements[3].SQL, "N'c'")
	for _, statement := range statements {
		require.NotContains(t, statement.SQL, "custom_length")
	}
}

func TestEnumChangeRejectsAmbiguousConstraintsBeforeCompiling(t *testing.T) {
	for _, rows := range [][]map[string]any{
		{{"name": "custom_membership", "definition": "state in ('a')", "single_column": true}},
		{{"name": "CK__items__state", "definition": "state in ('a')", "single_column": true, "is_system_named": true}, {"name": "CK__items__state1", "definition": "state in ('a')", "single_column": true, "is_system_named": true}},
		{{"name": "CK__items__state", "definition": "lower(state) in ('a')", "single_column": true, "is_system_named": true}},
	} {
		g := NewGrammar("")
		b := schema.NewBlueprint(nil, "", "tenant.items")
		b.Enum("state", []any{"a", "b"}).Change()
		statements, handled, err := g.CompileSchemaBlueprint(b, enumTestCatalog{rows: rows})
		require.True(t, handled)
		require.ErrorIs(t, err, errors.SchemaEnumConstraintAmbiguous)
		require.Empty(t, statements)
	}
	b := schema.NewBlueprint(nil, "", "items")
	b.Enum("state", []any{"a"}).Change()
	_, err := b.CompileStatements(NewGrammar(""))
	require.ErrorIs(t, err, errors.SchemaEnumCatalogRequired)
}

func TestUseCurrentChangeRegistersDefaultAfterTypeCompilation(t *testing.T) {
	b := schema.NewBlueprint(nil, "", "items")
	b.Timestamp("created_at", 4).UseCurrent().Change()
	statements, err := b.CompileStatements(NewGrammar(""))
	require.NoError(t, err)
	require.Len(t, statements, 3)
	require.Equal(t, "alter table [items] add default CURRENT_TIMESTAMP for [created_at]", statements[2].SQL)
}

func TestEnumChangeRejectsIncompatibleDataBeforeDDL(t *testing.T) {
	b := schema.NewBlueprint(nil, "", "tenant.items")
	b.Enum("state", []any{"closed"}).Change()
	g := NewGrammar("")
	require.True(t, g.SchemaExecutionPolicy(b).NestedTransaction)
	statements, handled, err := g.CompileSchemaBlueprint(b, enumTestCatalog{invalid: true})
	require.True(t, handled)
	require.ErrorIs(t, err, errors.SchemaEnumValuesNotAllowed)
	require.Empty(t, statements)
}
func TestComputedExplicitNullability(t *testing.T) {
	for _, nullable := range []bool{false, true} {
		b := schema.NewBlueprint(nil, "", "items")
		b.Create()
		b.Computed("n", "[v] + 1").Persisted().Nullable(nullable)
		statements, err := b.CompileStatements(NewGrammar(""))
		require.NoError(t, err)
		if nullable {
			require.NotContains(t, statements[0].SQL, "not null")
		} else {
			require.Contains(t, statements[0].SQL, "persisted not null")
		}
	}
	b := schema.NewBlueprint(nil, "", "items")
	b.Computed("n", "[v] + 1").Nullable(false)
	_, err := b.CompileStatements(NewGrammar(""))
	require.ErrorIs(t, err, errors.SchemaComputedNotNullRequiresPersisted)
}
func TestSchemaUnicodeLiterals(t *testing.T) {
	type label string
	for _, value := range []any{"中文🌏", label("O'Reilly"), ""} {
		require.True(t, len(schemaDefaultValue(value)) >= 3)
		require.Contains(t, schemaDefaultValue(value), "N'")
	}
	require.Equal(t, "N'O''Reilly'", schemaDefaultValue(label("O'Reilly")))
	require.Equal(t, "CURRENT_TIMESTAMP", schemaDefaultValue(schema.Expression("CURRENT_TIMESTAMP")))
	require.Equal(t, "'1'", schemaDefaultValue(true))
}
