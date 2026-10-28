package sqlserver

import (
	"fmt"
	"reflect"
	"regexp"
	"slices"
	"strconv"
	"strings"

	sq "github.com/Masterminds/squirrel"
	"github.com/gradii/framework/contracts/database/driver"
	"github.com/gradii/framework/database/query/core"
	"github.com/gradii/framework/database/query/grammars"
	"github.com/gradii/framework/database/schema"
	"github.com/gradii/framework/errors"
	"github.com/gradii/framework/support/convert"
	"github.com/spf13/cast"
)

var _ driver.Grammar = &Grammar{}

type Grammar struct {
	enumChanges       map[*driver.Command][]driver.SchemaStatement
	versionResolver   func() (string, error)
	attributeCommands []string
	modifiers         []func(driver.Blueprint, driver.ColumnDefinition) string
	prefix            string
	serials           []string
	wrap              *Wrap
}

func NewGrammar(prefix string) *Grammar {
	grammar := &Grammar{
		attributeCommands: []string{schema.CommandComment, schema.CommandDefault},
		prefix:            prefix,
		serials:           []string{"bigInteger", "integer", "mediumInteger", "smallInteger", "tinyInteger"},
		wrap:              NewWrap(prefix),
	}
	grammar.modifiers = []func(driver.Blueprint, driver.ColumnDefinition) string{
		grammar.ModifyDefault,
		grammar.ModifyIncrement,
		grammar.ModifyNullable,
	}

	return grammar
}

func (r *Grammar) CompileAdd(blueprint driver.Blueprint, command *driver.Command) string {
	return fmt.Sprintf("alter table %s add %s", r.wrap.Table(blueprint.GetTableName()), r.getColumn(blueprint, command.Column))
}

func (r *Grammar) CompileChange(blueprint driver.Blueprint, command *driver.Command) []string {
	return []string{
		r.CompileDropDefaultConstraint(blueprint, command),
		fmt.Sprintf("alter table %s alter column %s", r.wrap.Table(blueprint.GetTableName()), r.getColumn(blueprint, command.Column)),
	}
}

func (r *Grammar) CompileColumns(_, table string) (string, error) {
	schema, table, err := parseSchemaAndTable(table, "")
	if err != nil {
		return "", err
	}

	table = r.prefix + table

	newSchema := "schema_name()"
	if schema != "" {
		newSchema = r.wrap.Quote(schema)
	}

	return fmt.Sprintf(
		"select col.name, type.name as type_name, "+
			"col.max_length as length, col.precision as precision, col.scale as places, "+
			"col.is_nullable as nullable, def.definition as [default], "+
			"col.is_identity as autoincrement, col.collation_name as collation, "+
			"com.definition as [expression], is_persisted as [persisted], "+
			"cast(prop.value as nvarchar(max)) as comment "+
			"from sys.columns as col "+
			"join sys.types as type on col.user_type_id = type.user_type_id "+
			"join sys.objects as obj on col.object_id = obj.object_id "+
			"join sys.schemas as scm on obj.schema_id = scm.schema_id "+
			"left join sys.default_constraints def on col.default_object_id = def.object_id and col.object_id = def.parent_object_id "+
			"left join sys.extended_properties as prop on obj.object_id = prop.major_id and col.column_id = prop.minor_id and prop.name = 'MS_Description' "+
			"left join sys.computed_columns as com on col.column_id = com.column_id and col.object_id = com.object_id "+
			"where obj.type in ('U', 'V') and obj.name = %s and scm.name = %s "+
			"order by col.column_id", r.wrap.Quote(table), newSchema), nil
}

func (r *Grammar) CompileComment(_ driver.Blueprint, _ *driver.Command) string {
	return ""
}

func (r *Grammar) CompileCreate(blueprint driver.Blueprint) string {
	return fmt.Sprintf("create table %s (%s)", r.wrap.Table(blueprint.GetTableName()), strings.Join(r.getColumns(blueprint), ", "))
}

func (r *Grammar) CompileDefault(blueprint driver.Blueprint, command *driver.Command) string {
	if command.Column.IsChange() && command.Column.GetDefault() != nil {
		return fmt.Sprintf("alter table %s add default %s for %s",
			r.wrap.Table(blueprint.GetTableName()),
			schemaDefaultValue(command.Column.GetDefault()),
			r.wrap.Column(command.Column.GetName()),
		)
	}

	return ""
}

func (r *Grammar) CompileDrop(blueprint driver.Blueprint) string {
	return fmt.Sprintf("drop table %s", r.wrap.Table(blueprint.GetTableName()))
}

func (r *Grammar) CompileDropAllDomains(_ []string) string {
	return ""
}

func (r *Grammar) CompileDropAllForeignKeys() string {
	return `DECLARE @sql NVARCHAR(MAX) = N'';
            SELECT @sql += 'ALTER TABLE '
                + QUOTENAME(OBJECT_SCHEMA_NAME(parent_object_id)) + '.' + + QUOTENAME(OBJECT_NAME(parent_object_id))
                + ' DROP CONSTRAINT ' + QUOTENAME(name) + ';'
            FROM sys.foreign_keys;

            EXEC sp_executesql @sql;`
}

func (r *Grammar) CompileDropAllTables(_ string, _ []driver.Table) []string {
	return []string{
		r.CompileDropAllForeignKeys(),
		"EXEC sp_msforeachtable 'DROP TABLE ?'",
	}
}

func (r *Grammar) CompileDropAllTypes(_ string, _ []driver.Type) []string {
	return nil
}

func (r *Grammar) CompileDropAllViews(_ string, _ []driver.View) []string {
	return []string{`
DECLARE @sql NVARCHAR(MAX) = N'';
SELECT @sql += 'DROP VIEW ' + QUOTENAME(OBJECT_SCHEMA_NAME(object_id)) + '.' + QUOTENAME(name) + ';' FROM sys.views;
EXEC sp_executesql @sql;`,
	}
}

func (r *Grammar) CompileDropColumn(blueprint driver.Blueprint, command *driver.Command) []string {
	columns := r.wrap.Columns(command.Columns)

	dropExistingConstraintsSql := r.CompileDropDefaultConstraint(blueprint, command)

	return []string{
		fmt.Sprintf("%s alter table %s drop column %s", dropExistingConstraintsSql, r.wrap.Table(blueprint.GetTableName()), strings.Join(columns, ", ")),
	}
}

func (r *Grammar) CompileDropDefaultConstraint(blueprint driver.Blueprint, command *driver.Command) string {
	// TODO Add change logic
	columns := fmt.Sprintf("'%s'", strings.Join(command.Columns, "','"))
	if command.Column != nil && command.Column.IsChange() {
		columns = fmt.Sprintf("'%s'", command.Column.GetName())
	}

	table := r.wrap.Table(blueprint.GetTableName())
	tableName := r.wrap.Quote(table)

	return fmt.Sprintf("DECLARE @sql NVARCHAR(MAX) = '';"+
		"SELECT @sql += 'ALTER TABLE %s DROP CONSTRAINT ' + OBJECT_NAME([default_object_id]) + ';' "+
		"FROM sys.columns "+
		"WHERE [object_id] = OBJECT_ID(%s) AND [name] in (%s) AND [default_object_id] <> 0;"+
		"EXEC(@sql);", table, tableName, columns)
}

func (r *Grammar) CompileDropForeign(blueprint driver.Blueprint, command *driver.Command) string {
	return fmt.Sprintf("alter table %s drop constraint %s", r.wrap.Table(blueprint.GetTableName()), r.wrap.Column(command.Index))
}

func (r *Grammar) CompileDropFullText(_ driver.Blueprint, _ *driver.Command) string {
	return ""
}

func (r *Grammar) CompileDropIfExists(blueprint driver.Blueprint) string {
	table := r.wrap.Table(blueprint.GetTableName())

	return fmt.Sprintf("if object_id(%s, 'U') is not null drop table %s", r.wrap.Quote(table), table)
}

func (r *Grammar) CompileDropIndex(blueprint driver.Blueprint, command *driver.Command) string {
	return fmt.Sprintf("drop index %s on %s", r.wrap.Column(command.Index), r.wrap.Table(blueprint.GetTableName()))
}

func (r *Grammar) CompileDropPrimary(blueprint driver.Blueprint, command *driver.Command) string {
	return fmt.Sprintf("alter table %s drop constraint %s", r.wrap.Table(blueprint.GetTableName()), r.wrap.Column(command.Index))
}

func (r *Grammar) CompileDropUnique(blueprint driver.Blueprint, command *driver.Command) string {
	return r.CompileDropIndex(blueprint, command)
}

func (r *Grammar) CompileForeign(blueprint driver.Blueprint, command *driver.Command) string {
	sql := fmt.Sprintf("alter table %s add constraint %s foreign key (%s) references %s (%s)",
		r.wrap.Table(blueprint.GetTableName()),
		r.wrap.Column(command.Index),
		r.wrap.Columnize(command.Columns),
		r.wrap.Table(command.On),
		r.wrap.Columnize(command.References))
	if command.OnDelete != "" {
		sql += " on delete " + command.OnDelete
	}
	if command.OnUpdate != "" {
		sql += " on update " + command.OnUpdate
	}

	return sql
}

func (r *Grammar) CompileForeignKeys(schema, table string) string {
	newSchema := "schema_name()"
	if schema != "" {
		newSchema = r.wrap.Quote(schema)
	}

	return fmt.Sprintf(
		`SELECT 
			fk.name AS name, 
			string_agg(lc.name, ',') WITHIN GROUP (ORDER BY fkc.constraint_column_id) AS columns, 
			fs.name AS foreign_schema, 
			ft.name AS foreign_table, 
			string_agg(fc.name, ',') WITHIN GROUP (ORDER BY fkc.constraint_column_id) AS foreign_columns, 
			fk.update_referential_action_desc AS on_update, 
			fk.delete_referential_action_desc AS on_delete 
		FROM sys.foreign_keys AS fk 
		JOIN sys.foreign_key_columns AS fkc ON fkc.constraint_object_id = fk.object_id 
		JOIN sys.tables AS lt ON lt.object_id = fk.parent_object_id 
		JOIN sys.schemas AS ls ON lt.schema_id = ls.schema_id 
		JOIN sys.columns AS lc ON fkc.parent_object_id = lc.object_id AND fkc.parent_column_id = lc.column_id 
		JOIN sys.tables AS ft ON ft.object_id = fk.referenced_object_id 
		JOIN sys.schemas AS fs ON ft.schema_id = fs.schema_id 
		JOIN sys.columns AS fc ON fkc.referenced_object_id = fc.object_id AND fkc.referenced_column_id = fc.column_id 
		WHERE lt.name = %s AND ls.name = %s 
		GROUP BY fk.name, fs.name, ft.name, fk.update_referential_action_desc, fk.delete_referential_action_desc`,
		r.wrap.Quote(table),
		newSchema,
	)
}

func (r *Grammar) CompileFullText(_ driver.Blueprint, _ *driver.Command) string {
	return ""
}

func (r *Grammar) CompileIndex(blueprint driver.Blueprint, command *driver.Command) string {
	return fmt.Sprintf("create index %s on %s (%s)",
		r.wrap.Column(command.Index),
		r.wrap.Table(blueprint.GetTableName()),
		r.wrap.Columnize(command.Columns),
	)
}

func (r *Grammar) CompileIndexes(_, table string) (string, error) {
	schema, table, err := parseSchemaAndTable(table, "")
	if err != nil {
		return "", err
	}

	table = r.prefix + table

	newSchema := "schema_name()"
	if schema != "" {
		newSchema = r.wrap.Quote(schema)
	}

	return fmt.Sprintf(
		"select idx.name as name, string_agg(col.name, ',') within group (order by idxcol.key_ordinal) as columns, "+
			"idx.type_desc as [type], idx.is_unique as [unique], idx.is_primary_key as [primary] "+
			"from sys.indexes as idx "+
			"join sys.tables as tbl on idx.object_id = tbl.object_id "+
			"join sys.schemas as scm on tbl.schema_id = scm.schema_id "+
			"join sys.index_columns as idxcol on idx.object_id = idxcol.object_id and idx.index_id = idxcol.index_id "+
			"join sys.columns as col on idxcol.object_id = col.object_id and idxcol.column_id = col.column_id "+
			"where tbl.name = %s and scm.name = %s "+
			"group by idx.name, idx.type_desc, idx.is_unique, idx.is_primary_key",
		r.wrap.Quote(table),
		newSchema,
	), nil
}

func (r *Grammar) CompileJsonColumnsUpdate(values map[string]any) (map[string]any, error) {
	var (
		compiled = make(map[string]any)
		json     = App.GetJson()
	)
	for key, value := range values {
		if strings.Contains(key, "->") {
			segments := strings.SplitN(key, "->", 2)
			column, path := segments[0], strings.Trim(r.wrap.JsonPath(segments[1]), "'")

			val := reflect.ValueOf(value)
			if val.Kind() == reflect.Pointer {
				val = val.Elem()
			}

			if kind := val.Kind(); kind == reflect.Slice || kind == reflect.Array || kind == reflect.Map || kind == reflect.Struct {
				binding, err := json.Marshal(value)
				if err != nil {
					return nil, err
				}
				value = legacyRaw("json_query(?)", string(binding))
			} else if kind == reflect.Bool {
				var bit int
				if val.Bool() {
					bit = 1
				}
				value = legacyRaw(fmt.Sprintf("cast(%d as bit)", bit))
			} else if kind == reflect.Float64 || kind == reflect.Float32 {
				value = legacyRaw(r.compileDecimalCastExpr(val.Float()))
			}

			expr, ok := compiled[column]
			if !ok {
				expr = legacyRaw(r.wrap.Column(column))
			}

			compiled[column] = legacyRaw("json_modify(?,?,?)", expr, path, value)

			continue
		}

		compiled[key] = value
	}

	return compiled, nil
}

func (r *Grammar) CompileJsonContains(column string, value any, isNot bool) (string, []any, error) {
	field, path := r.wrap.JsonFieldAndPath(column)
	query := r.wrap.Not(fmt.Sprintf(`? in (select "value" from openjson(%s%s))`, field, path), isNot)

	if val := reflect.ValueOf(value); val.Kind() == reflect.Slice || val.Kind() == reflect.Array {
		values := make([]any, val.Len())
		queries := make([]string, val.Len())
		for i := 0; i < val.Len(); i++ {
			values[i] = val.Index(i).Interface()
			queries[i] = query
		}

		return strings.Join(queries, " AND "), values, nil
	}

	return query, []any{value}, nil
}

func (r *Grammar) CompileJsonContainsKey(column string, isNot bool) string {
	segments := strings.Split(column, "->")
	lastSegment := segments[len(segments)-1]
	segments = segments[:len(segments)-1]

	key := "'" + strings.ReplaceAll(lastSegment, "'", "''") + "'"
	if matches := regexp.MustCompile(`\[([0-9]+)]$`).FindStringSubmatch(lastSegment); len(matches) == 2 {
		segments = append(segments, strings.TrimSuffix(lastSegment, matches[0]))
		key = matches[1]
	}

	field, path := r.wrap.JsonFieldAndPath(strings.Join(segments, "->"))

	return r.wrap.Not(fmt.Sprintf(`%s in (select "key" from openjson(%s%s))`, key, field, path), isNot)
}

func (r *Grammar) CompileJsonLength(column string) string {
	field, path := r.wrap.JsonFieldAndPath(column)

	return fmt.Sprintf("(select count(*) from openjson(%s%s))", field, path)
}

func (r *Grammar) CompileJsonSelector(column string) string {
	field, path := r.wrap.JsonFieldAndPath(column)

	return fmt.Sprintf("json_value(%s%s)", field, path)
}

func (r *Grammar) CompileJsonValues(args ...any) []any {

	return args
}

func (r *Grammar) CompileLimit(builder sq.SelectBuilder, conditions *driver.Conditions) sq.SelectBuilder {
	if conditions.Limit == nil {
		return builder
	}

	return builder.Suffix("FETCH NEXT ? ROWS ONLY", *conditions.Limit)
}

func (r *Grammar) CompileLockForUpdate(builder sq.SelectBuilder, conditions *driver.Conditions) sq.SelectBuilder {
	if conditions.LockForUpdate != nil && *conditions.LockForUpdate {
		builder = builder.From(conditions.Table + " WITH (ROWLOCK, UPDLOCK, HOLDLOCK)")
	}

	return builder
}

func (r *Grammar) CompileOffset(builder sq.SelectBuilder, conditions *driver.Conditions) sq.SelectBuilder {
	if conditions.Offset == nil && conditions.Limit != nil {
		conditions.Offset = convert.Pointer[uint64](0)
	}
	if conditions.Offset != nil && conditions.Limit != nil {
		builder = builder.Suffix("OFFSET ? ROWS", *conditions.Offset)
	}

	return builder
}

func (r *Grammar) CompileOrderBy(builder sq.SelectBuilder, conditions *driver.Conditions) sq.SelectBuilder {
	if len(conditions.OrderBy) == 0 && conditions.Limit != nil {
		builder = builder.OrderBy("(select 0)")
	}

	return builder.OrderBy(conditions.OrderBy...)
}

func (r *Grammar) CompilePlaceholderFormat() driver.PlaceholderFormat {
	return sq.AtP
}

func (r *Grammar) CompilePrimary(blueprint driver.Blueprint, command *driver.Command) string {
	return fmt.Sprintf("alter table %s add constraint %s primary key (%s)",
		r.wrap.Table(blueprint.GetTableName()),
		r.wrap.Column(command.Index),
		r.wrap.Columnize(command.Columns))
}

func (r *Grammar) CompilePrune(database string) string {
	return fmt.Sprintf("dbcc shrinkdatabase (%s)", database)
}

func (r *Grammar) CompileInRandomOrder(builder sq.SelectBuilder, conditions *driver.Conditions) sq.SelectBuilder {
	if conditions.InRandomOrder != nil && *conditions.InRandomOrder {
		conditions.OrderBy = []string{"NEWID()"}
	}

	return builder
}

func (r *Grammar) CompileRename(blueprint driver.Blueprint, command *driver.Command) string {
	return fmt.Sprintf("sp_rename %s, %s", r.wrap.Quote(r.wrap.Table(blueprint.GetTableName())), r.wrap.Table(command.To))
}

func (r *Grammar) CompileRenameColumn(blueprint driver.Blueprint, command *driver.Command, _ []driver.Column) (string, error) {
	return fmt.Sprintf("sp_rename %s, %s, N'COLUMN'",
		r.wrap.Quote(r.wrap.Table(blueprint.GetTableName())+"."+r.wrap.Column(command.From)),
		r.wrap.Column(command.To),
	), nil
}

func (r *Grammar) CompileRenameIndex(blueprint driver.Blueprint, command *driver.Command, _ []driver.Index) []string {
	return []string{
		fmt.Sprintf("sp_rename %s, %s, N'INDEX'", r.wrap.Quote(r.wrap.Table(blueprint.GetTableName())+"."+r.wrap.Column(command.From)), r.wrap.Column(command.To)),
	}
}

func (r *Grammar) CompileSharedLock(builder sq.SelectBuilder, conditions *driver.Conditions) sq.SelectBuilder {
	if conditions.LockForUpdate != nil && *conditions.LockForUpdate {
		builder = builder.From(conditions.Table + " WITH (ROWLOCK, HOLDLOCK)")
	}

	return builder
}

func (r *Grammar) CompileTables(_ string) string {
	return "select t.name as name, schema_name(t.schema_id) as [schema], sum(u.total_pages) * 8 * 1024 as size " +
		"from sys.tables as t " +
		"join sys.partitions as p on p.object_id = t.object_id " +
		"join sys.allocation_units as u on u.container_id = p.hobt_id " +
		"group by t.name, t.schema_id " +
		"order by t.name"
}

func (r *Grammar) CompileTableComment(_ driver.Blueprint, _ *driver.Command) string {
	return ""
}

func (r *Grammar) CompileTypes() string {
	return ""
}

func (r *Grammar) CompileUnique(blueprint driver.Blueprint, command *driver.Command) string {
	return fmt.Sprintf("create unique index %s on %s (%s)",
		r.wrap.Column(command.Index),
		r.wrap.Table(blueprint.GetTableName()),
		r.wrap.Columnize(command.Columns))
}

func (r *Grammar) CompileVersion() string {
	return "SELECT SERVERPROPERTY('productversion') AS value;"
}

func (r *Grammar) CompileViews(_ string) string {
	return "select name, schema_name(v.schema_id) as [schema], definition from sys.views as v " +
		"inner join sys.sql_modules as m on v.object_id = m.object_id " +
		"order by name"
}

func (r *Grammar) GetAttributeCommands() []string {
	return r.attributeCommands
}

func (r *Grammar) ModifyDefault(_ driver.Blueprint, column driver.ColumnDefinition) string {
	if !column.IsChange() && column.GetDefault() != nil {
		return fmt.Sprintf(" default %s", schemaDefaultValue(column.GetDefault()))
	}

	return ""
}

func (r *Grammar) ModifyNullable(_ driver.Blueprint, column driver.ColumnDefinition) string {
	if column.GetNullable() {
		return " null"
	}

	return " not null"
}

func (r *Grammar) ModifyIncrement(blueprint driver.Blueprint, column driver.ColumnDefinition) string {
	if !column.IsChange() && slices.Contains(r.serials, column.GetType()) && column.GetAutoIncrement() {
		if blueprint.HasCommand("primary") {
			return " identity"
		}
		return " identity primary key"
	}

	return ""
}

func (r *Grammar) TypeBigInteger(_ driver.ColumnDefinition) string {
	return "bigint"
}

func (r *Grammar) TypeBoolean(_ driver.ColumnDefinition) string {
	return "bit"
}

func (r *Grammar) TypeChar(column driver.ColumnDefinition) string {
	return fmt.Sprintf("nchar(%d)", column.GetLength())
}

func (r *Grammar) TypeDate(_ driver.ColumnDefinition) string {
	return "date"
}

func (r *Grammar) TypeDateTime(column driver.ColumnDefinition) string {
	return r.TypeTimestamp(column)
}

func (r *Grammar) TypeDateTimeTz(column driver.ColumnDefinition) string {
	return r.TypeTimestampTz(column)
}

func (r *Grammar) TypeDecimal(column driver.ColumnDefinition) string {
	return fmt.Sprintf("decimal(%d, %d)", column.GetTotal(), column.GetPlaces())
}

func (r *Grammar) TypeDouble(_ driver.ColumnDefinition) string {
	return "double precision"
}

func (r *Grammar) TypeEnum(column driver.ColumnDefinition) string {
	return fmt.Sprintf(`nvarchar(255) check ("%s" in (%s))`, column.GetName(), strings.Join(r.wrap.Quotes(cast.ToStringSlice(column.GetAllowed())), ", "))
}

func (r *Grammar) TypeFloat(column driver.ColumnDefinition) string {
	precision := column.GetPrecision()
	if precision > 0 {
		return fmt.Sprintf("float(%d)", precision)
	}

	return "float"
}

func (r *Grammar) TypeInteger(_ driver.ColumnDefinition) string {
	return "int"
}

func (r *Grammar) TypeJson(_ driver.ColumnDefinition) string {
	return "nvarchar(max)"
}

func (r *Grammar) TypeJsonb(_ driver.ColumnDefinition) string {
	return "nvarchar(max)"
}

func (r *Grammar) TypeLongText(_ driver.ColumnDefinition) string {
	return "nvarchar(max)"
}

func (r *Grammar) TypeMediumInteger(_ driver.ColumnDefinition) string {
	return "int"
}

func (r *Grammar) TypeMediumText(_ driver.ColumnDefinition) string {
	return "nvarchar(max)"
}

func (r *Grammar) TypeSmallInteger(_ driver.ColumnDefinition) string {
	return "smallint"
}

func (r *Grammar) TypeString(column driver.ColumnDefinition) string {
	length := column.GetLength()
	if length > 0 {
		return fmt.Sprintf("nvarchar(%d)", length)
	}

	return "nvarchar(255)"
}

func (r *Grammar) TypeText(_ driver.ColumnDefinition) string {
	return "nvarchar(max)"
}

func (r *Grammar) TypeTime(column driver.ColumnDefinition) string {
	if column.GetPrecision() > 0 {
		return fmt.Sprintf("time(%d)", column.GetPrecision())
	} else {
		return "time"
	}
}

func (r *Grammar) TypeTimeTz(column driver.ColumnDefinition) string {
	return r.TypeTime(column)
}

func (r *Grammar) TypeTimestamp(column driver.ColumnDefinition) string {
	if column.GetUseCurrent() {
		column.Default(schema.Expression("CURRENT_TIMESTAMP"))
	}

	if column.GetPrecision() > 0 {
		return fmt.Sprintf("datetime2(%d)", column.GetPrecision())
	} else {
		return "datetime"
	}
}

func (r *Grammar) TypeTimestampTz(column driver.ColumnDefinition) string {
	if column.GetUseCurrent() {
		column.Default(schema.Expression("CURRENT_TIMESTAMP"))
	}

	if column.GetPrecision() > 0 {
		return fmt.Sprintf("datetimeoffset(%d)", column.GetPrecision())
	} else {
		return "datetimeoffset"
	}
}

func (r *Grammar) TypeTinyInteger(_ driver.ColumnDefinition) string {
	return "tinyint"
}

func (r *Grammar) TypeTinyText(_ driver.ColumnDefinition) string {
	return "nvarchar(255)"
}

func (r *Grammar) TypeUuid(_ driver.ColumnDefinition) string {
	return "uniqueidentifier"
}

func (r *Grammar) compileDecimalCastExpr(value float64) (string, string) {
	param := strconv.FormatFloat(value, 'f', -1, 64)
	parts := strings.Split(param, ".")
	intLen := len(parts[0])
	decLen := 0

	if len(parts) > 1 {
		decLen = len(strings.TrimRight(parts[1], "0"))
	}

	if decLen == 0 {
		decLen = 1
	}
	precision := intLen + decLen

	return fmt.Sprintf("cast(? as decimal(%d,%d))", precision, decLen), param
}

func (r *Grammar) getColumns(blueprint driver.Blueprint) []string {
	var columns []string
	for _, column := range blueprint.GetAddedColumns() {
		columns = append(columns, r.getColumn(blueprint, column))
	}

	return columns
}

func (r *Grammar) getColumn(blueprint driver.Blueprint, column driver.ColumnDefinition) string {
	sql := fmt.Sprintf("%s %s", r.wrap.Column(column.GetName()), schema.ColumnType(r, column))

	for _, modifier := range r.modifiers {
		sql += modifier(blueprint, column)
	}

	return sql
}

func parseSchemaAndTable(reference, defaultSchema string) (string, string, error) {
	if reference == "" {
		return "", "", errors.SchemaEmptyReferenceString
	}

	parts := strings.Split(reference, ".")
	if len(parts) > 2 {
		return "", "", errors.SchemaErrorReferenceFormat
	}

	schema := defaultSchema
	if len(parts) == 2 {
		schema = parts[0]
		parts = parts[1:]
	}

	table := parts[0]

	return schema, table, nil
}

func legacyRaw(value string, bindings ...any) core.Expression {
	grammar := grammars.NewBaseGrammar("")
	dialect := NewDialect()
	grammar.SetEscaper(func(value any, binary bool) string {
		if core.IsExpression(value) {
			return core.ExpressionValue(value)
		}
		result, _ := dialect.Escape(value, binary)
		return result
	})
	return core.Raw(grammar.SubstituteBindingsIntoRawSQL(value, bindings))
}

func (r *Grammar) SetVersionResolver(resolve func() (string, error)) { r.versionResolver = resolve }
