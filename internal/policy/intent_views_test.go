package policy_test

import (
	"strings"
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/policy"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage/storagetest"
)

func TestIntentReadsAreTenantScopedAndNameWhatTheyReadWhenNothingIsStored(t *testing.T) {
	t.Parallel()
	db := storagetest.OpenTemp(t)

	if _, err := policy.Intent(t.Context(), db.DB, "tenant", "int-1"); err == nil || !strings.Contains(err.Error(), "intent int-1") {
		t.Fatalf("Intent = %v, want an error naming the intent", err)
	}
	if intents, err := policy.DecisionIntents(t.Context(), db.DB, "tenant", "dec-1"); err != nil || len(intents) != 0 {
		t.Fatalf("DecisionIntents = %+v, %v; want none", intents, err)
	}
	_ = db.Close()
	if _, err := policy.DecisionIntents(t.Context(), db.DB, "tenant", "dec-1"); err == nil || !strings.Contains(err.Error(), "intents of decision dec-1") {
		t.Fatalf("DecisionIntents on a closed database = %v, want an error naming the decision", err)
	}
}
