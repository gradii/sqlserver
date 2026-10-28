package sqlserver

import (
	"github.com/gradii/framework/contracts/database/driver"
	"github.com/gradii/framework/database/query/processors"
	"github.com/gradii/framework/errors"
	"strings"
)

func (r *Grammar) CurrentSchemas(_ string) []string { return []string{"dbo"} }
func (r *Grammar) ResolveCurrentSchemas(info driver.SchemaConnectionInfo, catalog driver.SchemaCatalog) ([]string, error) {
	if info.Schema != "" {
		return []string{info.Schema}, nil
	}
	rows, err := catalog.Select(driver.SchemaStatement{SQL: "select schema_name() as name"})
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, nil
	}
	return []string{processors.MetadataString(rows[0]["name"])}, nil
}
func (r *Grammar) CompileSchemaMetadata(query driver.SchemaMetadataQuery) (driver.SchemaStatement, error) {
	switch query.Kind {
	case "hasTable":
		name := query.Table
		if query.Schema != "" {
			name = query.Schema + "." + name
		}
		return driver.SchemaStatement{SQL: "select (case when object_id(?, 'U') is null then 0 else 1 end) as [exists]", Bindings: []any{name}}, nil
	case "schemas":
		return driver.SchemaStatement{SQL: "select name, iif(schema_id = schema_id(), 1, 0) as [default] from sys.schemas where name not in ('information_schema', 'sys') and name not like 'db[_]%' order by name", Bindings: nil}, nil
	case "columns":
		return driver.SchemaStatement{SQL: "select col.name, type.name as type_name, case when type.name in ('nchar', 'nvarchar') and col.max_length > 0 then col.max_length / 2 else col.max_length end as length, case when type.name in ('time', 'datetime2', 'datetimeoffset') then col.scale else col.precision end as precision, col.scale as places, col.is_nullable as nullable, def.definition as [default], col.is_identity as autoincrement, col.collation_name as collation, com.definition as [expression], is_persisted as [persisted], cast(prop.value as nvarchar(max)) as comment from sys.columns as col join sys.types as type on col.user_type_id = type.user_type_id join sys.objects as obj on col.object_id = obj.object_id join sys.schemas as scm on obj.schema_id = scm.schema_id left join sys.default_constraints def on col.default_object_id = def.object_id and col.object_id = def.parent_object_id left join sys.extended_properties as prop on obj.object_id = prop.major_id and col.column_id = prop.minor_id and prop.name = 'MS_Description' left join sys.computed_columns as com on col.column_id = com.column_id and col.object_id = com.object_id where obj.type in ('U', 'V') and obj.name = ? and scm.name = coalesce(nullif(?, ''), schema_name()) order by col.column_id", Bindings: []any{query.Table, query.Schema}}, nil
	case "indexes":
		return driver.SchemaStatement{SQL: "select idx.name as name, string_agg(col.name, ',') within group (order by idxcol.key_ordinal) as columns, idx.type_desc as [type], idx.is_unique as [unique], idx.is_primary_key as [primary] from sys.indexes as idx join sys.tables as tbl on idx.object_id = tbl.object_id join sys.schemas as scm on tbl.schema_id = scm.schema_id join sys.index_columns as idxcol on idx.object_id = idxcol.object_id and idx.index_id = idxcol.index_id join sys.columns as col on idxcol.object_id = col.object_id and idxcol.column_id = col.column_id where idxcol.is_included_column = 0 and idx.name is not null and tbl.name = ? and scm.name = coalesce(nullif(?, ''), schema_name()) group by idx.name, idx.type_desc, idx.is_unique, idx.is_primary_key", Bindings: []any{query.Table, query.Schema}}, nil
	case "foreignKeys":
		return driver.SchemaStatement{SQL: "select fk.name as name, string_agg(lc.name, ',') within group (order by fkc.constraint_column_id) as columns, fs.name as foreign_schema, ft.name as foreign_table, string_agg(fc.name, ',') within group (order by fkc.constraint_column_id) as foreign_columns, fk.update_referential_action_desc as on_update, fk.delete_referential_action_desc as on_delete from sys.foreign_keys as fk join sys.foreign_key_columns as fkc on fkc.constraint_object_id = fk.object_id join sys.tables as lt on lt.object_id = fk.parent_object_id join sys.schemas as ls on lt.schema_id = ls.schema_id join sys.columns as lc on fkc.parent_object_id = lc.object_id and fkc.parent_column_id = lc.column_id join sys.tables as ft on ft.object_id = fk.referenced_object_id join sys.schemas as fs on ft.schema_id = fs.schema_id join sys.columns as fc on fkc.referenced_object_id = fc.object_id and fkc.referenced_column_id = fc.column_id where lt.name = ? and ls.name = coalesce(nullif(?, ''), schema_name()) group by fk.name, fs.name, ft.name, fk.update_referential_action_desc, fk.delete_referential_action_desc", Bindings: []any{query.Table, query.Schema}}, nil

	case "tables":
		filter, bindings := schemaPredicate("schema_name(t.schema_id)", query.Schemas, "1 = 1")
		return driver.SchemaStatement{SQL: "select t.name as name, schema_name(t.schema_id) as [schema], sum(u.total_pages) * 8 * 1024 as size, cast((select value from sys.extended_properties where class=1 and major_id=t.object_id and minor_id=0 and name='MS_Description') as nvarchar(max)) as comment from sys.tables t join sys.partitions p on p.object_id=t.object_id join sys.allocation_units u on u.container_id=p.hobt_id where t.is_ms_shipped=0 and t.name<>'sysdiagrams' and " + filter + " group by t.object_id, t.name, t.schema_id order by [schema],t.name", Bindings: bindings}, nil
	case "views":
		filter, bindings := schemaPredicate("schema_name(v.schema_id)", query.Schemas, "1 = 1")
		return driver.SchemaStatement{SQL: "select name, schema_name(v.schema_id) as [schema], definition from sys.views v join sys.sql_modules m on v.object_id=m.object_id where v.is_ms_shipped=0 and " + filter + " order by [schema],name", Bindings: bindings}, nil
	case "types":
		filter, bindings := schemaPredicate("schema_name(t.schema_id)", query.Schemas, "1 = 1")
		return driver.SchemaStatement{SQL: "select name, schema_name(schema_id) as [schema], name as [type] from sys.types t where is_user_defined=1 and is_table_type=0 and " + filter + " order by [schema],name", Bindings: bindings}, nil
	}
	return driver.SchemaStatement{}, errors.SchemaMetadataNotSupported.Args(query.Kind, Name)
}
func schemaPredicate(column string, schemas []string, fallback string) (string, []any) {
	if len(schemas) == 0 || (len(schemas) == 1 && schemas[0] == "") {
		return fallback, nil
	}
	args := make([]any, len(schemas))
	for i, value := range schemas {
		args[i] = value
	}
	if len(schemas) == 1 {
		return column + " = ?", args
	}
	return column + " in (" + strings.TrimSuffix(strings.Repeat("?, ", len(schemas)), ", ") + ")", args
}
