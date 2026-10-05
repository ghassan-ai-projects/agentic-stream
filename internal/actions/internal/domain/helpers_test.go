package domain

import "encoding/json"

func jsonUnmarshal(data []byte, target any) error {
	return json.Unmarshal(data, target) //nolint:wrapcheck // Test helper.
}
