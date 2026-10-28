package sqlserver

import (
	"testing"

	query "github.com/gradii/framework/database/query"
	"github.com/gradii/framework/database/query/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestQueryGrammarPreservesExplicitIdentifiers(t *testing.T) {
	builder := query.NewBuilder(NewQueryGrammar("tenant_")).From("InventoryEntry", "").Select("SKUCode", "DisplayLabel").Where("SKUCode", "sku-17")
	statement, bindings := builder.ToSelectSQL()
	require.NoError(t, builder.Error())
	assert.Equal(t, `select [SKUCode], [DisplayLabel] from [tenant_InventoryEntry] where [SKUCode] = ?`, statement)
	assert.Equal(t, []any{"sku-17"}, bindings)
}

// These cases also exercise the query runtime for the JSON cases in GrammarSuite.
func TestQueryGrammarCompileJsonColumnsUpdate(t *testing.T) {
	tests := []struct {
		name     string
		values   map[string]any
		sql      string
		bindings []any
		hasError bool
	}{
		{
			name:     "invalid values",
			values:   map[string]any{"data->invalid": map[string]any{"value": func() {}}},
			hasError: true,
		},
		{
			name:     "update boolean value",
			values:   map[string]any{"data->bool": true},
			sql:      `[data] = json_modify([data], '$."bool"', ?)`,
			bindings: []any{true, 17},
		},
		{
			name:     "update float value",
			values:   map[string]any{"data->float": 456.789},
			sql:      `[data] = json_modify([data], '$."float"', ?)`,
			bindings: []any{456.789, 17},
		},
		{
			name:     "update single json column",
			values:   map[string]any{"data->details": "details value"},
			sql:      `[data] = json_modify([data], '$."details"', ?)`,
			bindings: []any{"details value", 17},
		},
		{
			name:     "update single json column(with nested path)",
			values:   map[string]any{"data->details->subdetails[0]": "subdetails value"},
			sql:      `[data] = json_modify([data], '$."details"."subdetails"[0]', ?)`,
			bindings: []any{"subdetails value", 17},
		},
		{
			name:     "update multiple json columns",
			values:   map[string]any{"data->details": "details value", "data->info": "info value"},
			sql:      `[data] = json_modify(json_modify([data], '$."details"', ?), '$."info"', ?)`,
			bindings: []any{"details value", "info value", 17},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			builder := query.NewBuilder(NewQueryGrammar("goravel_")).From("users", "").Where("id", 17)
			statement, bindings := builder.ToUpdateSQL(tt.values)
			if tt.hasError {
				require.Error(t, builder.Error())
				return
			}
			require.NoError(t, builder.Error())
			assert.Equal(t, `update [goravel_users] set `+tt.sql+` where [id] = ?`, statement)
			assert.Equal(t, tt.bindings, bindings)
		})
	}
}

func TestQueryGrammarCompileJsonContains(t *testing.T) {
	tests := []struct {
		name     string
		column   string
		value    any
		not      bool
		sql      string
		bindings []any
	}{
		{"single path with single value", "data->details", "value1", false,
			`? in (select [value] from openjson([data], '$."details"'))`, []any{"value1"}},
		{"single path with multiple values", "data->details", []string{"value1", "value2"}, false,
			`? in (select [value] from openjson([data], '$."details"')) and ? in (select [value] from openjson([data], '$."details"'))`, []any{"value1", "value2"}},
		{"nested path with single value", "data->details->subdetails[0]", "value1", false,
			`? in (select [value] from openjson([data], '$."details"."subdetails"[0]'))`, []any{"value1"}},
		{"nested path with multiple values", "data->details[0]->subdetails", []string{"value1", "value2"}, false,
			`? in (select [value] from openjson([data], '$."details"[0]."subdetails"')) and ? in (select [value] from openjson([data], '$."details"[0]."subdetails"'))`, []any{"value1", "value2"}},
		{"with is not condition", "data->details", "value1", true,
			`not ? in (select [value] from openjson([data], '$."details"'))`, []any{"value1"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			builder := query.NewBuilder(NewQueryGrammar("goravel_")).From("users", "").WhereJsonContains(tt.column, tt.value, core.BooleanAnd, tt.not)
			statement, bindings := builder.ToSelectSQL()
			require.NoError(t, builder.Error())
			assert.Equal(t, `select * from [goravel_users] where `+tt.sql, statement)
			assert.Equal(t, tt.bindings, bindings)
		})
	}
}

func TestQueryGrammarCompileJsonContainsKey(t *testing.T) {
	tests := []struct {
		name   string
		column string
		not    bool
		sql    string
	}{
		{"single path", "data->details", false, `'details' in (select [key] from openjson([data]))`},
		{"single path with is not", "data->details", true, `not 'details' in (select [key] from openjson([data]))`},
		{"nested path", "data->details->subdetails", false, `'subdetails' in (select [key] from openjson([data], '$."details"'))`},
		{"nested path with array index", "data->details[0]->subdetails", false, `'subdetails' in (select [key] from openjson([data], '$."details"[0]'))`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			builder := query.NewBuilder(NewQueryGrammar("goravel_")).From("users", "").WhereJsonContainsKey(tt.column, core.BooleanAnd, tt.not)
			statement, bindings := builder.ToSelectSQL()
			require.NoError(t, builder.Error())
			assert.Equal(t, `select * from [goravel_users] where `+tt.sql, statement)
			assert.Empty(t, bindings)
		})
	}
}

func TestQueryGrammarCompileJsonLength(t *testing.T) {
	tests := []struct {
		name   string
		column string
		sql    string
	}{
		{"single path", "data->details", `(select count(*) from openjson([data], '$."details"')) > ?`},
		{"nested path", "data->details->subdetails", `(select count(*) from openjson([data], '$."details"."subdetails"')) > ?`},
		{"nested path with array index", "data->details[0]->subdetails", `(select count(*) from openjson([data], '$."details"[0]."subdetails"')) > ?`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			builder := query.NewBuilder(NewQueryGrammar("goravel_")).From("users", "").WhereJsonLength(tt.column, ">", 2, core.BooleanAnd)
			statement, bindings := builder.ToSelectSQL()
			require.NoError(t, builder.Error())
			assert.Equal(t, `select * from [goravel_users] where `+tt.sql, statement)
			assert.Equal(t, []any{2}, bindings)
		})
	}
}
