package domain

import "encoding/json"

const watchIDKey = "watch_id"

func InstallResult(watchID string) map[string]any {
	return map[string]any{"accepted": true, watchIDKey: watchID}
}

func InstalledWatchID(providerResult []byte) string {
	var result map[string]any
	if json.Unmarshal(providerResult, &result) != nil {
		return ""
	}
	watchID, _ := result[watchIDKey].(string)
	return watchID
}
