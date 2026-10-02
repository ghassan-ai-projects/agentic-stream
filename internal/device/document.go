package device

func documentString(document map[string]any, key string) string {
	value, _ := document[key].(string)
	return value
}

func documentInt64(document map[string]any, key string) int64 {
	value, _ := document[key].(float64)
	return int64(value)
}
