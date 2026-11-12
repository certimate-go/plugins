package utils

func String(a string) *string {
	return &a
}

func StringValue(a *string) string {
	if a == nil {
		return ""
	}
	return *a
}

func Int32(a int32) *int32 {
	return &a
}

func Int32Value(a *int32) int32 {
	if a == nil {
		return 0
	}
	return *a
}

func Int64(a int64) *int64 {
	return &a
}

func Int64Value(a *int64) int64 {
	if a == nil {
		return 0
	}
	return *a
}

func Bool(a bool) *bool {
	return &a
}

func BoolValue(a *bool) bool {
	if a == nil {
		return false
	}
	return *a
}

func DefaultInt32(reaNum, defaultNum *int32) *int32 {
	if reaNum == nil {
		return defaultNum
	}
	return reaNum
}

func DefaultInt64(reaNum, defaultNum *int64) *int64 {
	if reaNum == nil {
		return defaultNum
	}
	return reaNum
}

func DefaultBool(reaNum, defaultNum *bool) *bool {
	if reaNum == nil {
		return defaultNum
	}
	return reaNum
}

func DefaultString(reaStr, defaultStr *string) *string {
	if reaStr == nil {
		return defaultStr
	}
	return reaStr
}
