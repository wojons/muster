package dsl

import (
	"fmt"

	"go.starlark.net/starlark"
)

func ToStarlark(v interface{}) (starlark.Value, error) {
	switch val := v.(type) {
	case string:
		return starlark.String(val), nil
	case int:
		return starlark.MakeInt(val), nil
	case int64:
		return starlark.MakeInt64(val), nil
	case float64:
		return starlark.Float(val), nil
	case bool:
		return starlark.Bool(val), nil
	case []interface{}:
		list := starlark.NewList([]starlark.Value{})
		for _, item := range val {
			sv, err := ToStarlark(item)
			if err != nil {
				return nil, err
			}
			_ = list.Append(sv)
		}
		return list, nil
	case map[string]interface{}:
		dict := starlark.NewDict(len(val))
		for k, v := range val {
			sk := starlark.String(k)
			sv, err := ToStarlark(v)
			if err != nil {
				return nil, err
			}
			_ = dict.SetKey(sk, sv)
		}
		return dict, nil
	default:
		return nil, fmt.Errorf("unsupported type: %T", v)
	}
}

func FromStarlark(v starlark.Value) (interface{}, error) {
	switch val := v.(type) {
	case starlark.String:
		return string(val), nil
	case starlark.Int:
		return val.BigInt().Int64(), nil
	case starlark.Float:
		return float64(val), nil
	case starlark.Bool:
		return bool(val), nil
	case *starlark.List:
		result := make([]interface{}, 0, val.Len())
		for i := 0; i < val.Len(); i++ {
			item := val.Index(i)
			converted, err := FromStarlark(item)
			if err != nil {
				return nil, err
			}
			result = append(result, converted)
		}
		return result, nil
	case *starlark.Dict:
		result := make(map[string]interface{})
		for _, item := range val.Items() {
			key, _ := FromStarlark(item[0])
			value, _ := FromStarlark(item[1])
			result[key.(string)] = value
		}
		return result, nil
	default:
		return nil, fmt.Errorf("unsupported starlark type: %T", v)
	}
}

func ToStarlarkDict(m map[string]interface{}) (*starlark.Dict, error) {
	dict := starlark.NewDict(len(m))
	for k, v := range m {
		sk := starlark.String(k)
		sv, err := ToStarlark(v)
		if err != nil {
			return nil, err
		}
		_ = dict.SetKey(sk, sv)
	}
	return dict, nil
}

func FromStarlarkDict(d *starlark.Dict) map[string]interface{} {
	result := make(map[string]interface{})
	for _, item := range d.Items() {
		key, _ := FromStarlark(item[0])
		value, _ := FromStarlark(item[1])
		result[key.(string)] = value
	}
	return result
}
