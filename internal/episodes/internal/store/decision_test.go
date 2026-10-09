package store_test

import (
	"database/sql"
	"strings"
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/episodeledger"
	"github.com/ghassan-ai-projects/agentic-stream/internal/episodes/internal/store"
)

var decisionTime = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

func insertProposedDecision(t *testing.T, tx *sql.Tx, episodeID, decisionID string) {
	t.Helper()
	identity, err := episodeledger.StartAttempt(t.Context(), tx, episodeID, "att-"+decisionID, decisionTime)
	if err != nil {
		t.Fatal(err)
	}
	var situationID string
	var version int
	if err := tx.QueryRowContext(t.Context(), "SELECT situation_id, situation_version FROM episodes WHERE episode_id = ?", episodeID).Scan(&situationID, &version); err != nil {
		t.Fatal(err)
	}
	if err := store.InsertDecision(t.Context(), store.Join(tx), store.DecisionInsert{
		DecisionID: decisionID, EpisodeID: episodeID, AttemptID: identity.AttemptID, Fence: identity.Fence,
		SituationID: situationID, SituationVersion: version, RawJSON: []byte(`{"decision_id":"` + decisionID + `"}`),
		Digest: bytesOf(1), ValidationStatus: "proposed", ValidationJSON: []byte(`{}`), Now: decisionTime,
	}); err != nil {
		t.Fatal(err)
	}
}

func insertAnnotatedAcceptedDecision(t *testing.T, tx *sql.Tx, episodeID, decisionID string) error {
	t.Helper()
	insertProposedDecision(t, tx, episodeID, decisionID)
	if err := store.AnnotateRejectedDecision(t.Context(), store.Join(tx), decisionID, "schema_invalid"); err != nil {
		return err
	}
	return store.AcceptDecision(t.Context(), store.Join(tx), decisionID)
}

func TestDecisionIsInsertedProposedThenAnnotatedAndAccepted(t *testing.T) {
	t.Parallel()
	db := replayedStore(t)
	episodeID := admittedEpisodeID(t, db)
	if err := db.WithTx(t.Context(), func(tx *sql.Tx) error {
		return insertAnnotatedAcceptedDecision(t, tx, episodeID, "dec-1")
	}); err != nil {
		t.Fatal(err)
	}

	var status, reason string
	if err := db.QueryRowContext(t.Context(), "SELECT validation_status, rejection_reason FROM decisions WHERE decision_id = 'dec-1'").Scan(&status, &reason); err != nil {
		t.Fatal(err)
	}
	if status != "accepted" || reason != "schema_invalid" {
		t.Fatalf("decision = (%q, %q), want accepted and annotated schema_invalid", status, reason)
	}
}

func TestValidatedIntentIsStoredPendingForThePolicyPlane(t *testing.T) {
	t.Parallel()
	db := replayedStore(t)
	episodeID := admittedEpisodeID(t, db)
	situationID := scalar[string](t, db, "SELECT situation_id FROM episodes WHERE episode_id = ?", episodeID)
	version := scalar[int](t, db, "SELECT situation_version FROM episodes WHERE episode_id = ?", episodeID)
	if err := db.WithTx(t.Context(), func(tx *sql.Tx) error {
		insertProposedDecision(t, tx, episodeID, "dec-1")
		return store.InsertValidatedIntent(t.Context(), store.Join(tx), store.ValidatedIntentInsert{
			Intent: intentFixture(), DecisionID: "dec-1", TenantID: "default", SituationID: situationID, SituationVersion: version, Now: decisionTime,
		})
	}); err != nil {
		t.Fatal(err)
	}

	var status, intentType, risk string
	if err := db.QueryRowContext(t.Context(), "SELECT policy_status, intent_type, risk_class FROM intents WHERE intent_id = 'int-1'").Scan(&status, &intentType, &risk); err != nil {
		t.Fatal(err)
	}
	if status != "pending" || intentType != "create_maintenance_ticket" || risk != "R1" {
		t.Fatalf("intent = (%q, %q, %q), want pending create_maintenance_ticket R1", status, intentType, risk)
	}
}

func TestValidatedIntentWithAnUndecodableDigestIsRefusedBeforeAnyWrite(t *testing.T) {
	t.Parallel()
	db := replayedStore(t)
	intent := intentFixture()
	intent.Digest = "not-a-digest"

	err := db.WithTx(t.Context(), func(tx *sql.Tx) error {
		return store.InsertValidatedIntent(t.Context(), store.Join(tx), store.ValidatedIntentInsert{Intent: intent, DecisionID: "dec-1"})
	})

	if err == nil || !strings.Contains(err.Error(), "decode intent digest") {
		t.Fatalf("error = %v, want decode intent digest", err)
	}
	if got := scalar[int](t, db, "SELECT COUNT(*) FROM intents"); got != 0 {
		t.Fatalf("intents = %d, want 0", got)
	}
}

func TestDecisionsAreReadInOrdinalOrderWithTheirProvenance(t *testing.T) {
	t.Parallel()
	db := replayedStore(t)
	episodeID := admittedEpisodeID(t, db)
	if err := db.WithTx(t.Context(), func(tx *sql.Tx) error {
		return insertAnnotatedAcceptedDecision(t, tx, episodeID, "dec-b")
	}); err != nil {
		t.Fatal(err)
	}

	views, err := store.New(db).Decisions(t.Context(), episodeID)

	if err != nil || len(views) != 1 {
		t.Fatalf("Decisions = %v, %v; want one decision", views, err)
	}
	view := views[0]
	if view.DecisionID != "dec-b" || view.Ordinal != 1 || view.Fence != 1 || view.AttemptID != "att-dec-b" || view.SituationVersion != 1 ||
		view.ValidationStatus != "accepted" || view.RejectionReason != "schema_invalid" || !strings.HasPrefix(view.DecisionSHA256, "sha256:") {
		t.Fatalf("decision view = %+v", view)
	}
	none, err := store.New(db).Decisions(t.Context(), "epi-without-decisions")
	if err != nil || len(none) != 0 {
		t.Fatalf("Decisions of an episode without any = %v, %v; want none", none, err)
	}
}

func TestDecisionsAfterTheFirstTakeTheNextOrdinal(t *testing.T) {
	t.Parallel()
	db := replayedStore(t)
	episodeID := admittedEpisodeID(t, db)
	if err := db.WithTx(t.Context(), func(tx *sql.Tx) error {
		insertProposedDecision(t, tx, episodeID, "dec-1")
		return store.InsertDecision(t.Context(), store.Join(tx), store.DecisionInsert{
			DecisionID: "dec-2", EpisodeID: episodeID, AttemptID: "att-dec-1", Fence: 1,
			SituationID: scalar[string](t, db, "SELECT situation_id FROM episodes"), SituationVersion: 1,
			RawJSON: []byte(`{}`), Digest: bytesOf(2), ValidationStatus: "proposed", ValidationJSON: []byte(`{}`), Now: decisionTime,
		})
	}); err != nil {
		t.Fatal(err)
	}

	views, err := store.New(db).Decisions(t.Context(), episodeID)

	if err != nil || len(views) != 2 || views[0].DecisionID != "dec-1" || views[0].Ordinal != 1 || views[1].DecisionID != "dec-2" || views[1].Ordinal != 2 {
		t.Fatalf("decisions = %+v, %v; want dec-1 at ordinal 1 then dec-2 at ordinal 2", views, err)
	}
}
