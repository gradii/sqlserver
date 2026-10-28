package sqlserver

import (
	"math/big"
	"strings"
	"testing"

	query "github.com/gradii/framework/database/query"
	"github.com/gradii/framework/database/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestJSONNumberPathUsesNumericSQLWithoutLosingDecimalPrecision(t *testing.T) {
	for _, number := range []string{"0", "-0", "1", "-12.50", "1.2300e2", "1e-3", "9007199254740993", "99999999999999999999999999999999999999", "0.00000000000000000000000000000000000001"} {
		t.Run(number, func(t *testing.T) {
			builder := query.NewBuilder(NewQueryGrammar("")).From("docs", "")
			statement, bindings := builder.ToUpdateSQL(map[string]any{"data->n": types.JSON(number)})
			require.NoError(t, builder.Error())
			assert.Contains(t, statement, "cast(? as decimal(38,")
			require.Len(t, bindings, 1)
			expected, ok := new(big.Rat).SetString(number)
			require.True(t, ok)
			actual, ok := new(big.Rat).SetString(bindings[0].(string))
			require.True(t, ok)
			assert.Zero(t, expected.Cmp(actual), "the SQL numeric parameter must preserve the JSON value")
			assert.NotContains(t, strings.ToLower(bindings[0].(string)), "e", "SQL Server DECIMAL string conversion does not accept exponents")
		})
	}
	for _, number := range []string{"1e40", "1e-40"} {
		builder := query.NewBuilder(NewQueryGrammar("")).From("docs", "")
		statement, bindings := builder.ToUpdateSQL(map[string]any{"data->n": types.JSON(number)})
		require.NoError(t, builder.Error())
		assert.Contains(t, statement, "cast(? as float)")
		assert.Equal(t, []any{number}, bindings)
	}
}
