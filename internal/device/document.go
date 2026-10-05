package device

func documentString(document map[string]any, key string) string {
	value, _ := document[key].(string)
	return value
}
