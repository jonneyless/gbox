package gbox

import "github.com/mitchellh/mapstructure"

func StructToMap(obj any) map[string]any {
	var result map[string]any
	err := mapstructure.Decode(obj, &result)
	if err != nil {
		return nil
	}
	return result
}
