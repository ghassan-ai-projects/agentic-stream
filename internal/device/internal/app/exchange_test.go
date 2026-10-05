package app

import (
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/device/internal/domain"
)

func TestCachedReplyIsIsolatedFromCallerMutation(t *testing.T) {
	t.Parallel()
	code := "expired"
	session := &Session{receipts: map[string]cachedExchange{
		"key": {commandIdentity: "digest", exchange: Exchange{
			Receipt: domain.Receipt{RejectCode: &code, Document: map[string]any{"reject_code": "expired"}},
			Result:  domain.Result{ErrorCode: &code, Document: map[string]any{"error_code": "expired"}},
		}},
	}}
	first, _, err := session.cachedExchange("key", "digest")
	if err != nil {
		t.Fatal(err)
	}
	*first.Receipt.RejectCode = "caller changed code"
	*first.Result.ErrorCode = "caller changed code"
	first.Receipt.Document["reject_code"] = "caller changed document"
	first.Result.Document["error_code"] = "caller changed document"
	second, _, err := session.cachedExchange("key", "digest")
	if err != nil || *second.Receipt.RejectCode != "expired" || *second.Result.ErrorCode != "expired" ||
		second.Receipt.Document["reject_code"] != "expired" || second.Result.Document["error_code"] != "expired" {
		t.Fatalf("cached reply was mutated: %+v, error=%v", second, err)
	}
}
