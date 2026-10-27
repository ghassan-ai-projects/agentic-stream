package app_test

import (
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/testsupport/plantwirefake"
)

func TestTheHostRemembersTheKindDigestsABirthAnnounced(t *testing.T) {
	t.Parallel()
	host := startHost(t, simulatedHost)
	if born := host.host.BornKindDigests(); len(born) != 0 {
		t.Fatalf("born kinds before any birth = %v, want none", born)
	}
	bornNode(t, host, "s-1")
	kinds, _ := plantwirefake.Fixture(t, ".", "NBIRTH.json")["kinds"].([]any)
	born := host.host.BornKindDigests()
	for _, raw := range kinds {
		kind, _ := raw.(map[string]any)
		if name, _ := kind["kind"].(string); born[name] != kind["kind_digest"] {
			t.Fatalf("born digest of %s = %q, want %v", name, born[name], kind["kind_digest"])
		}
	}
	if len(born) != len(kinds) || len(kinds) == 0 {
		t.Fatalf("born kinds = %v, want the %d kinds the birth announced", born, len(kinds))
	}
}
