package utils

import (
	"reflect"
	"strings"
)

type IgnoreFieldOf func(obj interface{}, field reflect.StructField, value reflect.Value) bool

func StructToMap(structObj interface{}, ignoreFieldOf IgnoreFieldOf) map[string]string {
	structType := reflect.TypeOf(structObj)
	if structType.Kind() == reflect.Ptr {
		structType = structType.Elem()
	}
	structValue := reflect.ValueOf(structObj)
	if structValue.Kind() == reflect.Ptr {
		structValue = structValue.Elem()
	}
	params := make(map[string]string)
	for i := 0; i < structType.NumField(); i++ {
		field := structType.Field(i)
		value := structValue.FieldByName(field.Name)
		if ignoreFieldOf != nil && ignoreFieldOf(structObj, field, value) {
			continue
		}
		tag := field.Tag.Get("json")
		omitempty := false
		name := field.Name
		if len(tag) > 0 {
			items := strings.Split(tag, ",")
			name = strings.TrimSpace(items[0])
			omitempty = len(items) > 1 && "omitempty" == strings.TrimSpace(items[1])
		}
		if omitempty && ValueIsEmpty(value) {
			continue
		}
		params[name] = ConvertInterfaceToString(value.Interface())
	}
	return params
}

func MergeMap(mObj ...map[string]interface{}) map[string]interface{} {
	newMap := make(map[string]interface{})
	for _, m := range mObj {
		for k, v := range m {
			vValue := reflect.ValueOf(v)
			if vValue.Kind() == reflect.Ptr {
				if !vValue.IsNil() {
					newMap[k] = v
				}
			} else {
				newMap[k] = v
			}
		}
	}
	return newMap
}

func Merge(sources ...interface{}) map[string]interface{} {
	res := make(map[string]interface{})
	for _, source := range sources {
		sourceValue := reflect.ValueOf(source)
		if sourceValue.Kind() == reflect.Ptr {
			sourceValue = sourceValue.Elem()
		}
		sourceType := reflect.TypeOf(source)
		if sourceType.Kind() == reflect.Ptr {
			sourceType = sourceType.Elem()
		}
		for i := 0; i < sourceType.NumField(); i++ {
			field := sourceType.Field(i)
			if IsSet(sourceValue.Field(i).Interface()) {
				res[field.Name] = sourceValue.Field(i).Interface()
			}
		}
	}
	return res
}

func ValueIsEmpty(value reflect.Value) bool {
	return (value.Kind() == reflect.Ptr && value.IsNil()) || (value.Kind() != reflect.Ptr && value.IsZero())
}
