package domain

// DocumentString projects a decoded JSON string field; an absent or non-string
// field is the empty string.
func DocumentString(document map[string]any, key string) string {
	value, _ := document[key].(string)
	return value
}

// DocumentInt projects a decoded JSON number field as an integer, without
// string coercion; an absent or non-number field is zero.
func DocumentInt(document map[string]any, key string) int {
	value, _ := document[key].(float64)
	return int(value)
}
