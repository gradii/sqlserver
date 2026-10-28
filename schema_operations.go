package sqlserver

import (
	"github.com/gradii/framework/contracts/database/driver"
	"github.com/gradii/framework/errors"
	"strings"
)

func (r *Grammar) ForeignKeyStateQuery() string { return "" }
func (r *Grammar) SchemaOperationPolicy(operation driver.SchemaOperation) driver.SchemaExecutionPolicy {
	return driver.SchemaExecutionPolicy{Transactional: strings.HasPrefix(operation.Kind, "dropAll")}
}
func (r *Grammar) CompileSchemaOperation(operation driver.SchemaOperation) ([]driver.SchemaStatement, error) {
	var statements []driver.SchemaStatement
	var sql string
	switch operation.Kind {
	case "foreignKeys":
		action := "NOCHECK"
		if operation.Enabled {
			action = "WITH CHECK CHECK"
		}
		sql = "declare @sql nvarchar(max) = N''; select @sql += N'alter table ' + quotename(schema_name(schema_id)) + N'.' + quotename(name) + N' " + action + " constraint all;' from sys.tables where is_ms_shipped=0; exec sp_executesql @sql;"
	case "createDatabase":
		sql = "create database " + r.ddlWrap().Value(operation.Name)
		if operation.Collation != "" {
			sql += " collate " + operation.Collation
		}
	case "dropDatabase":
		sql = "if db_id(" + r.ddlWrap().Quote(operation.Name) + ") is not null drop database " + r.ddlWrap().Value(operation.Name)
	case "dropAllTables":
		for _, table := range operation.Tables {
			name := qualifiedName(table.Schema, table.Name)
			statements = append(statements, driver.SchemaStatement{SQL: "declare @sql nvarchar(max) = N''; select @sql += N'alter table ' + quotename(object_schema_name(parent_object_id)) + N'.' + quotename(object_name(parent_object_id)) + N' drop constraint ' + quotename(name) + N';' from sys.foreign_keys where parent_object_id=object_id(?) or referenced_object_id=object_id(?); exec sp_executesql @sql;", Bindings: []any{name, name}})
		}
		for _, table := range operation.Tables {
			statements = append(statements, driver.SchemaStatement{SQL: "drop table " + r.quoteQualifiedName(qualifiedName(table.Schema, table.Name))})
		}
	case "dropAllViews":
		for _, view := range operation.Views {
			statements = append(statements, driver.SchemaStatement{SQL: "drop view " + r.quoteQualifiedName(qualifiedName(view.Schema, view.Name))})
		}
	case "dropAllTypes":
		for _, kind := range operation.Types {
			statements = append(statements, driver.SchemaStatement{SQL: "drop type " + r.quoteQualifiedName(qualifiedName(kind.Schema, kind.Name))})
		}
	default:
		return nil, errors.SchemaFeatureNotSupported.Args(Name, operation.Kind)
	}
	if sql != "" {
		statements = append(statements, driver.SchemaStatement{SQL: sql})
	}
	return statements, nil
}
func qualifiedName(schema, name string) string {
	if schema != "" {
		return schema + "." + name
	}
	return name
}
func (r *Grammar) quoteQualifiedName(value string) string {
	parts := strings.Split(value, ".")
	for i, part := range parts {
		parts[i] = r.ddlWrap().Value(part)
	}
	return strings.Join(parts, ".")
}
