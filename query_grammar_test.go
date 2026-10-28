package sqlserver

import (
	"testing"

	query "github.com/gradii/framework/database/query"
	"github.com/gradii/framework/database/query/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestQueryGrammarOutputClauseAndJSONBindings(t *testing.T) {
	grammar := NewQueryGrammar("app_")
	builder := query.NewBuilder(grammar).From("values archive", "")
	statement, bindings := builder.ToInsertGetIdSQL(core.InsertValues{{Column: "name", Value: query.Raw("' returning output inserted. '")}, {Column: "age", Value: 7}}, "record_id")
	assert.Equal(t, "insert into [app_values archive] ([name], [age]) output inserted.[record_id] values (' returning output inserted. ', ?)", statement)
	assert.Equal(t, []any{7}, bindings)
	builder = query.NewBuilder(grammar).From("users", "").Where("id", 3)
	statement, bindings = builder.ToUpdateSQL(core.UpdateValues{{Column: "data->a", Value: []any{1, 2}}, {Column: "name", Value: "alice"}, {Column: "data->b", Value: true}})
	require.NoError(t, builder.Error())
	assert.Equal(t, "update [app_users] set [data] = json_modify(json_modify([data], '$.\"a\"', json_query(?)), '$.\"b\"', ?), [name] = ? where [id] = ?", statement)
	assert.Equal(t, []any{"[1,2]", true, "alice", 3}, bindings)
}
func TestQueryGrammarUsesApplyTopAndLockHints(t *testing.T) {
	builder := query.NewBuilder(NewQueryGrammar("")).From("users", "").Select("id").Limit(2).Lock(true).JoinLateral("select 1 as value", "counts", "left")
	assert.Equal(t, "select top 2 [id] from [users] with(rowlock,updlock,holdlock) outer apply (select 1 as value) as [counts]", builder.ToSQL())
	assert.Equal(t, "select * from [users] order by (SELECT 0) offset 2 rows fetch next 3 rows only", query.NewBuilder(NewQueryGrammar("")).From("users", "").Offset(2).Limit(3).ToSQL())
}
