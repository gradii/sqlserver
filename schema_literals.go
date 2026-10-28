package sqlserver

import (
	"reflect"
	"strings"

	"github.com/gradii/framework/database/schema"
	"github.com/spf13/cast"
)

// SQL Server converts unprefixed literals through the database code page before
// assigning them to nvarchar. Keep expressions intact and quote text as Unicode.
func schemaDefaultValue(value any) string {
	switch value.(type) {
	case schema.Expression, interface{ SQL() string }, interface{ GetValue() string }:
		return schema.ColumnDefaultValue(value)
	}
	if value != nil && reflect.TypeOf(value).Kind() == reflect.String {
		return "N'" + strings.ReplaceAll(cast.ToString(value), "'", "''") + "'"
	}
	return schema.ColumnDefaultValue(value)
}
