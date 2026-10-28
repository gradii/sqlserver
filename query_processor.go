package sqlserver

import (
	"fmt"
	contractsquery "github.com/gradii/framework/contracts/database/query"
	"github.com/gradii/framework/database/query/processors"
	"strings"
)

func NewQueryProcessor() *QueryProcessor { return &QueryProcessor{} }

// QueryProcessor owns SQL Server's insert-ID retrieval, as in Laravel. go-mssqldb cannot use
// PDO's lastInsertId API, so the grammar emits OUTPUT and this processor reads its row on the writer.
type QueryProcessor struct{ processors.BaseProcessor }

func (p *QueryProcessor) ProcessInsertGetID(connection contractsquery.Connection, statement string, values []any, sequence string) (any, error) {
	if sequence == "" {
		sequence = "id"
	}

	return connection.SelectFromWriteConnection(statement, values, sequence)
}

// ProcessColumns ports QueryProcessor::processColumns.
func (p *QueryProcessor) ProcessColumns(results []contractsquery.Row, sql ...string) []contractsquery.Row {
	columns := make([]contractsquery.Row, 0, len(results))
	for _, result := range results {
		typ := processors.MetadataString(result["type_name"])
		switch typ {
		case "binary", "varbinary", "char", "varchar", "nchar", "nvarchar":
			if processors.MetadataInt(result["length"]) == -1 {
				typ += "(max)"
			} else {
				typ = fmt.Sprintf("%s(%d)", typ, processors.MetadataInt(result["length"]))
			}
		case "decimal", "numeric":
			typ = fmt.Sprintf("%s(%d,%d)", typ, processors.MetadataInt(result["precision"]), processors.MetadataInt(result["places"]))
		case "float", "datetime2", "datetimeoffset", "time":
			// Laravel exposes catalog precision here, including for temporal types.
			typ = fmt.Sprintf("%s(%d)", typ, processors.MetadataInt(result["precision"]))
		}
		var generation any
		if processors.MetadataString(result["expression"]) != "" {
			kind := "virtual"
			if processors.MetadataBool(result["persisted"]) {
				kind = "stored"
			}
			generation = contractsquery.Row{"type": kind, "expression": result["expression"]}
		}
		columns = append(columns, contractsquery.Row{
			"name": result["name"], "type_name": result["type_name"], "type": typ,
			"collation": result["collation"], "nullable": processors.MetadataBool(result["nullable"]), "default": result["default"],
			"auto_increment": processors.MetadataBool(result["autoincrement"]),
			"comment":        processors.NullableComment(result["comment"]), "generation": generation,
		})
	}
	return columns
}

func (p *QueryProcessor) ProcessIndexes(results []contractsquery.Row) []contractsquery.Row {
	indexes := make([]contractsquery.Row, 0, len(results))
	for _, r := range results {
		indexes = append(indexes, contractsquery.Row{"name": strings.ToLower(processors.MetadataString(r["name"])), "columns": processors.MetadataColumns(r["columns"]), "type": strings.ToLower(processors.MetadataString(r["type"])), "unique": processors.MetadataBool(r["unique"]), "primary": processors.MetadataBool(r["primary"])})
	}
	return indexes
}
func (p *QueryProcessor) ProcessForeignKeys(results []contractsquery.Row) []contractsquery.Row {
	keys := make([]contractsquery.Row, 0, len(results))
	for _, r := range results {
		keys = append(keys, contractsquery.Row{"name": r["name"], "columns": processors.MetadataColumns(r["columns"]), "foreign_schema": r["foreign_schema"], "foreign_table": r["foreign_table"], "foreign_columns": processors.MetadataColumns(r["foreign_columns"]), "on_update": strings.ToLower(strings.ReplaceAll(processors.MetadataString(r["on_update"]), "_", " ")), "on_delete": strings.ToLower(strings.ReplaceAll(processors.MetadataString(r["on_delete"]), "_", " "))})
	}
	return keys
}
