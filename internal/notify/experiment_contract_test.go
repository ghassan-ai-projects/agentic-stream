package notify_test

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"testing"
)

// Tamoz vendors the notification goldens byte for byte
// (gems/tamoz-stream/contracts/notification-goldens-v1.json) and the
// real-world-sensor experiment runs Tamoz against this runtime's SSE stream.
// Change the goldens only together with that copy. See
// docs/unfinished-work-review-2026-10-08/EXPERIMENT_COMPATIBILITY.md (E9).
const notificationGoldensSHA256 = "4ddf669678dd8c10053986c07879fe85e92d6fe4dbb6e553a45f1328fa923f03"

func TestExperimentNotificationGoldensAreUnchanged(t *testing.T) {
	t.Parallel()
	data, err := os.ReadFile("internal/domain/contracts/notification-goldens-v1.json")
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(data)
	if got := hex.EncodeToString(sum[:]); got != notificationGoldensSHA256 {
		t.Errorf("notification goldens changed (sha256 %s); update Tamoz's vendored copy in the same change, then this pin", got)
	}
}
