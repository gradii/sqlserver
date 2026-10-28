package sqlserver

import (
	"encoding/json"
	"strconv"
	"strings"

	"github.com/gradii/framework/database/query/core"
)

// SQL Server JSON updates extend Laravel's grammar to preserve the framework's JSON write API.
// JSON_VALUE can read a selector, but an assignment must target the column and use JSON_MODIFY.
// Multiple paths in one column are nested because SQL Server rejects duplicate SET assignments.
func (g *QueryGrammar) CompileUpdateColumns(b core.Query, values core.UpdateValues) string {
	merged := g.groupJsonColumnsForUpdate(values)
	parts := make([]string, 0, len(merged))
	for _, value := range merged {
		if group, ok := value.Value.(sqlServerJsonUpdate); ok {
			parts = append(parts, g.compileJsonUpdateGroup(value.Column, group))
			continue
		}
		parts = append(parts, g.Wrap(value.Column)+" = "+g.Parameter(value.Value))
	}

	return strings.Join(parts, ", ")
}

// CompileJsonUpdateColumn renders a single JSON path assignment.
func (g *QueryGrammar) CompileJsonUpdateColumn(key string, value any) string {
	column, _, _ := strings.Cut(key, "->")
	return g.compileJsonUpdateGroup(column, sqlServerJsonUpdate{{Column: key, Value: value}})
}

// groupJsonColumnsForUpdate keeps each JSON column at its first selector's position. Both SQL
// compilation and binding preparation use this order, including when ordinary assignments intervene.
func (g *QueryGrammar) groupJsonColumnsForUpdate(values core.UpdateValues) core.UpdateValues {
	merged := make(core.UpdateValues, 0, len(values))
	positions := make(map[string]int)
	for _, value := range values {
		if !g.IsJsonSelector(value.Column) {
			merged = append(merged, value)
			continue
		}

		column, _, _ := strings.Cut(value.Column, "->")
		if index, ok := positions[column]; ok {
			merged[index].Value = append(merged[index].Value.(sqlServerJsonUpdate), value)
			continue
		}
		positions[column] = len(merged)
		merged = append(merged, core.UpdateValue{Column: column, Value: sqlServerJsonUpdate{value}})
	}

	return merged
}

func (g *QueryGrammar) compileJsonUpdateGroup(column string, group sqlServerJsonUpdate) string {
	field := g.Wrap(column)
	expression := field
	for _, selector := range group {
		_, path := g.JsonFieldAndPath(selector.Column)
		value := g.Parameter(selector.Value)
		if core.IsArrayValue(selector.Value) {
			// JSON_QUERY prevents SQL Server from storing the encoded document as a quoted string.
			value = g.CompileJsonValueCast(value)
		} else if number, ok := selector.Value.(json.Number); ok {
			_, typ := sqlServerJSONNumber(number)
			value = "cast(" + value + " as " + typ + ")"
		}
		expression = "json_modify(" + expression + path + ", " + value + ")"
	}

	return field + " = " + expression
}

type sqlServerJsonUpdate []core.UpdateValue

// JSON numbers decoded with UseNumber must not become quoted strings in JSON_MODIFY. DECIMAL
// preserves exact values that fit SQL Server's 38 digits; other magnitudes use its floating type.
// The cast type is derived from bounded integers, and the number itself remains a SQL parameter.
func sqlServerJSONNumber(number json.Number) (string, string) {
	text := string(number)
	coefficient, exponentText, hasExponent := strings.Cut(strings.ToLower(text), "e")
	exponent := 0
	if hasExponent {
		var err error
		exponent, err = strconv.Atoi(exponentText)
		if err != nil || exponent < -38 || exponent > 38 {
			return text, "float"
		}
	}
	negative := strings.HasPrefix(coefficient, "-")
	coefficient = strings.TrimPrefix(coefficient, "-")
	integer, fraction, _ := strings.Cut(coefficient, ".")
	digits := strings.TrimLeft(integer+fraction, "0")
	if digits == "" {
		return "0", "decimal(38, 0)"
	}
	scale := len(fraction) - exponent
	for scale > 0 && strings.HasSuffix(digits, "0") {
		digits = strings.TrimSuffix(digits, "0")
		scale--
	}
	if scale < 0 {
		if len(digits)-scale > 38 {
			return text, "float"
		}
		digits += strings.Repeat("0", -scale)
		scale = 0
	}
	if scale > 38 || len(digits) > 38 {
		return text, "float"
	}
	if scale > 0 {
		if len(digits) <= scale {
			digits = strings.Repeat("0", scale-len(digits)+1) + digits
		}
		split := len(digits) - scale
		digits = digits[:split] + "." + digits[split:]
	}
	if negative {
		digits = "-" + digits
	}
	return digits, "decimal(38, " + strconv.Itoa(scale) + ")"
}
