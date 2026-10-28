package sqlserver

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/gradii/framework/contracts/database/driver"
	"github.com/gradii/framework/database/schema"
	"github.com/gradii/framework/errors"
)

var _ driver.SchemaCompiler = (*Grammar)(nil)
var _ driver.SchemaBlueprintPlanner = (*Grammar)(nil)

func describeColumn(column driver.ColumnDefinition) driver.ColumnDescription {
	if describer, ok := column.(driver.ColumnDescriber); ok {
		return describer.DescribeColumn()
	}
	return driver.ColumnDescription{}
}
func describeBlueprint(blueprint driver.Blueprint) driver.BlueprintDescription {
	if describer, ok := blueprint.(driver.BlueprintDescriber); ok {
		return describer.DescribeBlueprint()
	}
	return driver.BlueprintDescription{Table: blueprint.GetTableName(), Columns: blueprint.GetAddedColumns(), Commands: blueprint.GetCommands()}
}
func schemaExpression(value any) (string, error) {
	switch value := value.(type) {
	case nil:
		return "", nil
	case string:
		return value, nil
	case schema.Expression:
		return string(value), nil
	case interface{ SQL() string }:
		return value.SQL(), nil
	case interface{ GetValue() string }:
		return value.GetValue(), nil
	default:
		return "", errors.DatabaseUnsupportedType.Args(fmt.Sprintf("%T", value), "SQL expression")
	}
}
func (r *Grammar) ddlWrap() *schema.Wrap {
	wrap := schema.NewWrap(r.prefix)
	wrap.SetValueWrapper(func(value string) string {
		if value == "*" {
			return value
		}
		return "[" + strings.ReplaceAll(value, "]", "]]") + "]"
	})
	return wrap
}
func (r *Grammar) ddlTable(blueprint driver.Blueprint) string {
	if describeBlueprint(blueprint).Temporary {
		return r.ddlWrap().Value("#" + r.prefix + blueprint.GetTableName())
	}
	return r.ddlWrap().Table(blueprint.GetTableName())
}
func (r *Grammar) schemaColumnType(column driver.ColumnDefinition) (string, error) {
	options := describeColumn(column)
	if options.VirtualAs != nil || options.StoredAs != nil {
		value := options.VirtualAs
		if options.StoredAs != nil {
			value = options.StoredAs
		}
		expression, err := schemaExpression(value)
		return "as (" + expression + ")", err
	}
	switch column.GetType() {
	case "raw":
		return options.RawDefinition, nil
	case "binary":
		if column.GetLength() > 0 {
			name := "varbinary"
			if options.Fixed {
				name = "binary"
			}
			return fmt.Sprintf("%s(%d)", name, column.GetLength()), nil
		}
		return "varbinary(max)", nil
	case "year":
		return "integer", nil
	case "ipAddress":
		return "nvarchar(45)", nil
	case "macAddress":
		return "nvarchar(17)", nil
	case "set", "vector":
		return "", errors.SchemaFeatureNotSupported.Args(Name, column.GetType()+" columns")
	case "computed":
		expression, err := schemaExpression(options.Expression)
		return "as (" + expression + ")", err
	case "geometry", "geography":
		return column.GetType(), nil
	case "enum":
		if len(column.GetAllowed()) == 0 {
			return "", errors.SchemaEnumValuesRequired.Args(column.GetName())
		}
		return "nvarchar(255)", nil
	default:
		return schema.ColumnType(r, column), nil
	}
}
func (r *Grammar) schemaColumn(blueprint driver.Blueprint, column driver.ColumnDefinition) (string, error) {
	kind, err := r.schemaColumnType(column)
	if err != nil {
		return "", err
	}
	options := describeColumn(column)
	definition := r.ddlWrap().Value(column.GetName()) + " " + kind
	if column.GetType() == "computed" || options.VirtualAs != nil || options.StoredAs != nil {
		notNull := options.Nullable != nil && !*options.Nullable
		if notNull && (!options.Persisted && options.StoredAs == nil || column.IsChange()) {
			return "", errors.SchemaComputedNotNullRequiresPersisted
		}
		if options.Persisted || options.StoredAs != nil {
			definition += " persisted"
			if notNull {
				definition += " not null"
			}
		}
		return definition, nil
	}
	if options.Collation != "" {
		definition += " collate " + options.Collation
	}
	definition += r.ModifyNullable(blueprint, column) + r.ModifyDefault(blueprint, column)
	increment := r.ModifyIncrement(blueprint, column)
	if options.StartingValue != nil && !column.IsChange() {
		increment = strings.Replace(increment, " identity", " identity("+strconv.FormatUint(*options.StartingValue, 10)+",1)", 1)
	}
	definition += increment
	if column.GetType() == "enum" && !column.IsChange() {
		definition += " constraint " + r.ddlWrap().Value(r.enumName(blueprint, column.GetName())) + " check (" + r.enumCheck(column) + ")"
	}
	return definition, nil
}
func (r *Grammar) CompileSchemaCommand(blueprint driver.Blueprint, command *driver.Command) ([]driver.SchemaStatement, bool, error) {
	if statements, ok := r.enumChanges[command]; ok {
		return statements, true, nil
	}
	wrap := r.ddlWrap()
	table := r.ddlTable(blueprint)
	var statement string
	switch command.Name {
	case schema.CommandCreate:
		columns := make([]string, 0)
		for _, column := range describeBlueprint(blueprint).Columns {
			definition, err := r.schemaColumn(blueprint, column)
			if err != nil {
				return nil, true, err
			}
			columns = append(columns, definition)
		}
		statement = "create table " + table + " (" + strings.Join(columns, ", ") + ")"
	case schema.CommandAdd:
		if command.Column.IsChange() && command.Column.GetType() == "enum" && r.enumChanges == nil {
			return nil, true, errors.SchemaEnumCatalogRequired.Args(command.Column.GetName())
		}
		options := describeColumn(command.Column)
		if command.Column.IsChange() && command.Column.GetType() == "computed" {
			if options.Nullable != nil && !*options.Nullable {
				return nil, true, errors.SchemaComputedNotNullRequiresPersisted
			}
			action := "drop"
			if options.Persisted {
				action = "add"
			}
			statement = "alter table " + table + " alter column " + wrap.Value(command.Column.GetName()) + " " + action + " persisted"
			break
		}
		definition, err := r.schemaColumn(blueprint, command.Column)
		if err != nil {
			return nil, true, err
		}
		if command.Column.IsChange() {
			return []driver.SchemaStatement{r.dropDefaults(blueprint, []string{command.Column.GetName()}), {SQL: "alter table " + table + " alter column " + definition}}, true, nil
		}
		statement = "alter table " + table + " add " + definition
	case schema.CommandDrop:
		statement = "drop table " + table
	case schema.CommandDropIfExists:
		statement = "if object_id(" + wrap.Quote(table) + ", 'U') is not null drop table " + table
	case schema.CommandDropColumn:
		return []driver.SchemaStatement{r.dropDefaults(blueprint, command.Columns), {SQL: "alter table " + table + " drop column " + wrap.Columnize(command.Columns)}}, true, nil
	case schema.CommandRename:
		return []driver.SchemaStatement{{SQL: "exec sp_rename ?, ?", Bindings: []any{table, r.prefix + command.To}}}, true, nil
	case schema.CommandRenameColumn:
		return []driver.SchemaStatement{{SQL: "exec sp_rename ?, ?, 'COLUMN'", Bindings: []any{table + "." + wrap.Value(command.From), command.To}}}, true, nil
	case schema.CommandRenameIndex:
		return []driver.SchemaStatement{{SQL: "exec sp_rename ?, ?, 'INDEX'", Bindings: []any{table + "." + wrap.Value(command.From), command.To}}}, true, nil
	case schema.CommandPrimary:
		statement = "alter table " + table + " add constraint " + wrap.Value(command.Index) + " primary key (" + wrap.Columnize(command.Columns) + ")"
	case schema.CommandIndex, schema.CommandUnique, "rawIndex", "spatialIndex":
		kind := ""
		if command.Name == schema.CommandUnique {
			kind = "unique "
		}
		if command.Name == "spatialIndex" {
			kind = "spatial "
		}
		columns := wrap.Columnize(command.Columns)
		if command.Name == "rawIndex" {
			columns = command.Value
		}
		statement = "create " + kind + "index " + wrap.Value(command.Index) + " on " + table + " (" + columns + ")"
	case schema.CommandDropIndex, schema.CommandDropUnique, "dropSpatialIndex":
		statement = "drop index " + wrap.Value(command.Index) + " on " + table
	case schema.CommandDropPrimary, schema.CommandDropForeign:
		statement = "alter table " + table + " drop constraint " + wrap.Value(command.Index)
	case schema.CommandForeign:
		statement = "alter table " + table + " add constraint " + wrap.Value(command.Index) + " foreign key (" + wrap.Columnize(command.Columns) + ") references " + wrap.Table(command.On) + " (" + wrap.Columnize(command.References) + ")"
		if command.OnDelete != "" {
			statement += " on delete " + command.OnDelete
		}
		if command.OnUpdate != "" {
			statement += " on update " + command.OnUpdate
		}
	case schema.CommandDefault:
		if command.Column.IsChange() && command.Column.GetDefault() != nil {
			statement = "alter table " + table + " add default " + schemaDefaultValue(command.Column.GetDefault()) + " for " + wrap.Value(command.Column.GetName())
		}
	case schema.CommandComment:
		comment := describeColumn(command.Column).Comment
		if comment != nil {
			return []driver.SchemaStatement{r.commentStatement(blueprint, command.Column.GetName(), *comment)}, true, nil
		}
	case schema.CommandTableComment:
		return []driver.SchemaStatement{r.commentStatement(blueprint, "", command.Value)}, true, nil
	case schema.CommandFullText, schema.CommandDropFullText:
		return nil, true, errors.SchemaFeatureNotSupported.Args(Name, "fulltext indexes")
	default:
		return nil, false, nil
	}
	if statement == "" {
		return nil, true, nil
	}
	return []driver.SchemaStatement{{SQL: statement}}, true, nil
}
func (r *Grammar) dropDefaults(blueprint driver.Blueprint, columns []string) driver.SchemaStatement {
	bindings := []any{r.ddlTable(blueprint)}
	for _, column := range columns {
		bindings = append(bindings, column)
	}
	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(columns)), ",")
	return driver.SchemaStatement{SQL: "declare @sql nvarchar(max) = N''; select @sql += N'alter table ' + quotename(object_schema_name(c.object_id)) + N'.' + quotename(object_name(c.object_id)) + N' drop constraint ' + quotename(object_name(c.default_object_id)) + N';' from sys.columns c where c.object_id = object_id(?) and c.name in (" + placeholders + ") and c.default_object_id <> 0; exec sp_executesql @sql;", Bindings: bindings}
}
func (r *Grammar) commentStatement(blueprint driver.Blueprint, column, comment string) driver.SchemaStatement {
	schemaName, table, _ := parseSchemaAndTable(blueprint.GetTableName(), "")
	table = r.prefix + table
	statement := "declare @schema sysname = coalesce(nullif(?, ''), schema_name()), @table sysname = ?, @column sysname = ?, @description nvarchar(4000) = ?; "
	level := "@level0type=N'SCHEMA', @level0name=@schema, @level1type=N'TABLE', @level1name=@table"
	minor := "0"
	if column != "" {
		level += ", @level2type=N'COLUMN', @level2name=@column"
		minor = "columnproperty(object_id(quotename(@schema)+'.'+quotename(@table)), @column, 'ColumnId')"
	}
	condition := "exists (select 1 from sys.extended_properties where class=1 and major_id=object_id(quotename(@schema)+'.'+quotename(@table)) and minor_id=" + minor + " and name=N'MS_Description')"
	if comment == "" {
		statement += "if " + condition + " exec sys.sp_dropextendedproperty @name=N'MS_Description', " + level
	} else {
		statement += "if " + condition + " exec sys.sp_updateextendedproperty @name=N'MS_Description', @value=@description, " + level + "; else exec sys.sp_addextendedproperty @name=N'MS_Description', @value=@description, " + level
	}
	return driver.SchemaStatement{SQL: statement, Bindings: []any{schemaName, table, column, comment}}
}
func (r *Grammar) CompileSchemaStartingValues(driver.Blueprint) ([]driver.SchemaStatement, error) {
	return nil, nil
}
func (r *Grammar) SchemaExecutionPolicy(blueprint driver.Blueprint) driver.SchemaExecutionPolicy {
	policy := driver.SchemaExecutionPolicy{Transactional: true}
	if blueprint != nil {
		for _, command := range blueprint.GetCommands() {
			if command.Name == schema.CommandAdd && command.Column.IsChange() && command.Column.GetType() == "enum" {
				policy.NestedTransaction = true
			}
		}
	}
	return policy
}
