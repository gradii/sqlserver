package sqlserver

import (
	"database/sql/driver"
	"reflect"

	"github.com/gradii/framework/database/query/core"
	"github.com/gradii/framework/errors"
)

// PrepareJsonContains supports SQL Server scalar membership and our existing
// flat-list extension. Nested collections have no structural containment
// semantics here and must never reach AddBinding's list expansion.
func (g *QueryGrammar) PrepareJsonContains(value any) (any, any, error) {
	var err error
	value, err = core.NormalizeJSONMembershipValue(value)
	if err != nil {
		return nil, nil, err
	}
	if !core.IsArrayValue(value) {
		scalar, err := jsonContainsScalar(value)
		return scalar, jsonContainsBinding(scalar), err
	}
	rv := reflect.ValueOf(value)
	if rv.Kind() == reflect.Map {
		return nil, nil, errors.QueryJsonContainsValueNotSupported.Args(value)
	}
	items := core.ToAnySlice(value)
	normalized, bindings := make([]any, len(items)), make([]any, len(items))
	for i, item := range items {
		scalar, err := jsonContainsScalar(item)
		if err != nil {
			return nil, nil, err
		}
		normalized[i], bindings[i] = scalar, jsonContainsBinding(scalar)
	}
	return normalized, bindings, nil
}

func jsonContainsScalar(value any) (any, error) {
	var err error
	value, err = core.NormalizeJSONMembershipValue(value)
	if err != nil {
		return nil, err
	}
	if core.IsExpression(value) || core.IsArrayValue(value) {
		return nil, errors.QueryJsonContainsValueNotSupported.Args(value)
	}
	converted, err := driver.DefaultParameterConverter.ConvertValue(value)
	if err != nil {
		return nil, err
	}
	if _, ok := value.(driver.Valuer); ok {
		return converted, nil
	}
	if value != nil && (reflect.TypeOf(value).Kind() == reflect.Pointer || reflect.TypeOf(value).Kind() == reflect.Bool) {
		return converted, nil
	}
	return value, nil
}

func jsonContainsBinding(value any) any {
	if b, ok := value.(bool); ok {
		if b {
			return "true"
		}
		return "false"
	}
	return value
}
