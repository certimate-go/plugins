package utils

import (
	"encoding/json"
	"strconv"
	"strings"
)

func ConvertInterfaceToString(value interface{}) string {
	if value == nil {
		return ""
	}
	switch value.(type) {
	case int:
		return strconv.Itoa(value.(int))
	case string:
		return value.(string)
	case uint:
		return strconv.Itoa(int(value.(uint)))
	case int32:
		return strconv.Itoa(int(value.(int32)))
	case uint32:
		return strconv.Itoa(int(value.(uint32)))
	case int64:
		return strconv.FormatInt(value.(int64), 10)
	case uint64:
		return strconv.FormatUint(value.(uint64), 10)
	case float32:
		return strconv.FormatFloat(float64(value.(float32)), 'f', -1, 64)
	case float64:
		return strconv.FormatFloat(value.(float64), 'f', -1, 64)
	case int8:
		return strconv.Itoa(int(value.(int8)))
	case uint8:
		return strconv.Itoa(int(value.(uint8)))
	case int16:
		return strconv.Itoa(int(value.(int16)))
	case uint16:
		return strconv.Itoa(int(value.(uint16)))
	case []byte:
		return string(value.([]byte))
	default:
		b, err := json.Marshal(value)
		if err != nil {
			return ""
		}
		return strings.ReplaceAll(strings.Trim(string(b[:]), "[]"), "\"", "")
	}
}
