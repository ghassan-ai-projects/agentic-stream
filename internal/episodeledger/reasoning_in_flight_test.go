package episodeledger_test

import (
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/episodeledger"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage/storagetest"
)

func TestReasoningInFlightIsReadFromADatabaseHandleThroughTheFacade(t *testing.T) {
	t.Parallel()
	db := storagetest.OpenTempWithoutForeignKeys(t)
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	if inFlight, err := episodeledger.ReasoningInFlight(t.Context(), db.DB, now); err != nil || inFlight {
		t.Fatalf("empty ledger: inFlight=%v err=%v", inFlight, err)
	}
	admitEpisode(t, db, admission("e1", "sit-1"))
	if inFlight, err := episodeledger.ReasoningInFlight(t.Context(), db.DB, now); err != nil || !inFlight {
		t.Fatalf("admitted episode: inFlight=%v err=%v", inFlight, err)
	}
}
