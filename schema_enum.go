package sqlserver

import (
	"crypto/sha256"
	"fmt"
	"strings"

	"github.com/gradii/framework/contracts/database/driver"
	"github.com/gradii/framework/database/schema"
	"github.com/gradii/framework/errors"
	"github.com/spf13/cast"
)

func enumConstraintName(table, column string) string {
	sum := sha256.Sum256([]byte(table + "\x00" + column))
	return fmt.Sprintf("gradii_enum_%x", sum[:12])
}
func (r *Grammar) enumName(blueprint driver.Blueprint, column string) string {
	_, table, _ := parseSchemaAndTable(blueprint.GetTableName(), "")
	return enumConstraintName(r.prefix+table, column)
}
func (r *Grammar) enumCheck(column driver.ColumnDefinition) string {
	return enumCheckValue(r.ddlWrap().Value(column.GetName()), column)
}
func enumCheckValue(expression string, column driver.ColumnDefinition) string {
	values := make([]string, len(column.GetAllowed()))
	for i, value := range column.GetAllowed() {
		values[i] = schemaDefaultValue(fmt.Sprint(value))
	}
	return expression + " in (" + strings.Join(values, ", ") + ")"
}

func (r *Grammar) CompileSchemaBlueprint(blueprint driver.Blueprint, catalog driver.SchemaCatalog) ([]driver.SchemaStatement, bool, error) {
	if blueprint == nil || r.enumChanges != nil {
		return nil, false, nil
	}
	var commands []*driver.Command
	for _, command := range blueprint.GetCommands() {
		if command.Name == schema.CommandAdd && !command.ShouldBeSkipped && command.Column.IsChange() && command.Column.GetType() == "enum" {
			commands = append(commands, command)
		}
	}
	if len(commands) == 0 {
		return nil, false, nil
	}
	if catalog == nil {
		return nil, true, errors.SchemaEnumCatalogRequired.Args(commands[0].Column.GetName())
	}
	compiler, ok := blueprint.(interface {
		CompileStatements(driver.Grammar) ([]driver.SchemaStatement, error)
	})
	if !ok {
		return nil, true, errors.SchemaFeatureNotSupported.Args(Name, "enum planning for this blueprint")
	}
	namespace, table, err := catalog.ResolveTable(blueprint.GetTableName())
	if err != nil {
		return nil, true, err
	}
	// Replacements belong to this compilation; the shared grammar and the
	// ordering of unrelated blueprint commands remain untouched.
	planned := *r
	planned.enumChanges = make(map[*driver.Command][]driver.SchemaStatement)
	for _, command := range commands {
		column := command.Column
		if len(column.GetAllowed()) == 0 {
			return nil, true, errors.SchemaEnumValuesRequired.Args(column.GetName())
		}
		rows, err := catalog.Select(driver.SchemaStatement{
			SQL:      "select c.name, c.definition, c.is_system_named, case when c.parent_column_id <> 0 or (select count(distinct d.referenced_minor_id) from sys.sql_expression_dependencies d where d.referencing_id = c.object_id and d.referenced_id = c.parent_object_id and d.referenced_minor_id > 0) = 1 then 1 else 0 end as single_column from sys.check_constraints c join sys.tables t on t.object_id = c.parent_object_id join sys.schemas n on n.schema_id = t.schema_id join sys.columns a on a.object_id = t.object_id where t.name = ? and n.name = coalesce(nullif(?, ''), schema_name()) and a.name = ? and (c.parent_column_id = a.column_id or exists (select 1 from sys.sql_expression_dependencies d where d.referencing_id = c.object_id and d.referenced_id = t.object_id and d.referenced_minor_id = a.column_id))",
			Bindings: []any{table, namespace, column.GetName()},
		})
		if err != nil {
			return nil, true, err
		}
		constraint, err := enumConstraint(rows, table, column.GetName())
		if err != nil {
			return nil, true, err
		}
		// SQL Server rolls back the entire transaction when ADD CHECK rejects
		// existing data, including any caller-owned transaction. Validate under
		// serializable update locks before DDL so a rejected change is recoverable
		// through our savepoint and concurrent writes cannot invalidate the check.
		converted := "convert(nvarchar(max), " + r.ddlWrap().Value(column.GetName()) + ")"
		comparison := converted
		if collation := describeColumn(column).Collation; collation != "" {
			comparison += " collate " + collation
		}
		invalid := "not (" + enumCheckValue(comparison, column) + ") or datalength(" + converted + ") > 510"
		if !column.GetNullable() {
			invalid += " or " + r.ddlWrap().Value(column.GetName()) + " is null"
		}
		violations, err := catalog.Select(driver.SchemaStatement{SQL: "select top 1 1 as invalid from " + r.ddlTable(blueprint) + " with (updlock, holdlock) where " + invalid})
		if err != nil {
			return nil, true, err
		}
		if len(violations) != 0 {
			return nil, true, errors.SchemaEnumValuesNotAllowed.Args(column.GetName(), table)
		}
		var statements []driver.SchemaStatement
		if constraint != "" {
			statements = append(statements, driver.SchemaStatement{SQL: "alter table " + r.ddlTable(blueprint) + " drop constraint " + r.ddlWrap().Value(constraint)})
		}
		changed, _, err := planned.CompileSchemaCommand(blueprint, command)
		if err != nil {
			return nil, true, err
		}
		statements = append(statements, changed...)
		statements = append(statements, driver.SchemaStatement{SQL: "alter table " + r.ddlTable(blueprint) + " with check add constraint " + r.ddlWrap().Value(r.enumName(blueprint, column.GetName())) + " check (" + r.enumCheck(column) + ")"})
		planned.enumChanges[command] = statements
	}
	statements, err := compiler.CompileStatements(&planned)
	return statements, true, err
}

// Only framework names and SQL Server's system-generated inline-check names
// establish ownership. A custom membership check is not ours to remove.
func enumConstraint(rows []map[string]any, table, column string) (string, error) {
	found := ""
	for _, row := range rows {
		name := cast.ToString(row["name"])
		marked := isEnumConstraintName(name)
		owned := marked || cast.ToBool(row["is_system_named"])
		membership := enumMembership(cast.ToString(row["definition"]), column)
		if marked && !membership {
			return "", errors.SchemaEnumConstraintAmbiguous.Args(column, table)
		}
		if owned && membership {
			if !cast.ToBool(row["single_column"]) || found != "" {
				return "", errors.SchemaEnumConstraintAmbiguous.Args(column, table)
			}
			found = name
		} else if membership {
			return "", errors.SchemaEnumConstraintAmbiguous.Args(column, table)
		}
	}
	if found == "" && len(rows) != 0 {
		return "", errors.SchemaEnumConstraintAmbiguous.Args(column, table)
	}
	return found, nil
}
func isEnumConstraintName(name string) bool {
	if len(name) != len("gradii_enum_")+24 || !strings.HasPrefix(name, "gradii_enum_") {
		return false
	}
	for _, c := range strings.TrimPrefix(name, "gradii_enum_") {
		if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f') {
			return false
		}
	}
	return true
}
