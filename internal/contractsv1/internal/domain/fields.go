package domain

func DocumentString(document map[string]any, key string) string {
	value, _ := document[key].(string)
	return value
}

func DocumentInt(document map[string]any, key string) int {
	value, _ := document[key].(float64)
	return int(value)
}
