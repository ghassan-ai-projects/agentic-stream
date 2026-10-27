package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/episodes"
	"github.com/ghassan-ai-projects/agentic-stream/internal/playbook/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage/storagetest"
)

var storeNow = time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)

func recommendation(ruleID string) domain.Recommendation {
	return domain.Recommendation{RuleID: ruleID, RuleVersion: 1, PlaybookDigest: "sha256:pb", DiagnosisCode: "wear",
		Intent: "service", Preset: "wear", Parameters: map[string]any{"order": "wear"}}
}

func inTx(t *testing.T, s Store, use func(*Tx) error) {
	t.Helper()
	if err := s.WithTx(t.Context(), use); err != nil {
		t.Fatal(err)
	}
}

func TestFiresCountTowardTheRateCapAndQueueTheirAudits(t *testing.T) {
	t.Parallel()
	s := New(storagetest.OpenTemp(t))
	first := domain.Fire{FireID: "rfire_e1", TenantID: "t", EpisodeID: "e1", SituationID: "s1", SituationVersion: 1,
		Recommendation: recommendation("r"), AuditDue: true, FiredAt: storeNow}
	second := first
	second.FireID, second.EpisodeID, second.AuditDue, second.FiredAt = "rfire_e2", "e2", false, storeNow.Add(-2*time.Hour)
	inTx(t, s, func(tx *Tx) error {
		for _, fire := range []domain.Fire{first, second} {
			if err := tx.RecordFire(t.Context(), fire); err != nil {
				return err
			}
		}
		return nil
	})
	inTx(t, s, func(tx *Tx) error {
		fires, err := tx.FiresSince(t.Context(), "t", storeNow.Add(-time.Hour))
		if err != nil || fires[domain.RuleKey{ID: "r", Version: 1}] != 1 {
			t.Fatalf("fires in the last hour = %v, %v; want one", fires, err)
		}
		due, err := tx.DueAudits(t.Context(), "t")
		if err != nil || len(due) != 1 || due[0].FireID != "rfire_e1" || due[0].Recommendation.Preset != "wear" {
			t.Fatalf("due audits = %+v, %v; want the audited fire", due, err)
		}
		return tx.MarkAuditQueued(t.Context(), "rfire_e1")
	})
	inTx(t, s, func(tx *Tx) error {
		if due, err := tx.DueAudits(t.Context(), "t"); err != nil || len(due) != 0 {
			t.Fatalf("due audits after queueing = %+v, %v; want none", due, err)
		}
		return nil
	})
}

func TestAnAuditEvaluationIsBoundSettledAndNoLongerPending(t *testing.T) {
	t.Parallel()
	s := New(storagetest.OpenTemp(t))
	fire := domain.Fire{FireID: "rfire_e1", TenantID: "t", EpisodeID: "e1", SituationID: "s1", SituationVersion: 1,
		Recommendation: recommendation("r"), AuditDue: true, FiredAt: storeNow}
	audit := domain.NewAuditEvaluation(fire, "sch-1")
	inTx(t, s, func(tx *Tx) error {
		if err := tx.RecordFire(t.Context(), fire); err != nil {
			return err
		}
		if err := tx.RecordEvaluation(t.Context(), audit, storeNow); err != nil {
			return err
		}
		found, ok, err := tx.PendingAudit(t.Context(), "sch-1")
		if err != nil || !ok || found.EvaluationID != audit.EvaluationID || found.Recommendation.RuleID != "r" {
			t.Fatalf("pending audit = %+v %v %v", found, ok, err)
		}
		return tx.BindEvaluationEpisode(t.Context(), audit.EvaluationID, "e2")
	})
	inTx(t, s, func(tx *Tx) error {
		pending, err := tx.PendingEvaluations(t.Context(), "t")
		if err != nil || len(pending) != 1 || pending[0].EpisodeID != "e2" || pending[0].Kind != domain.KindAudit {
			t.Fatalf("pending evaluations = %+v, %v", pending, err)
		}
		return tx.SettleEvaluation(t.Context(), audit.EvaluationID, domain.VerdictDisagree, &domain.Observed{Intent: "other"}, storeNow)
	})
	inTx(t, s, func(tx *Tx) error {
		if _, ok, err := tx.PendingAudit(t.Context(), "sch-1"); err != nil || ok {
			t.Fatalf("a settled audit is still pending: %v %v", ok, err)
		}
		return nil
	})
}

func TestOpeningACircuitSwitchesTheRuleOffAndAlertsOnce(t *testing.T) {
	t.Parallel()
	db := storagetest.OpenTemp(t)
	s := New(db)
	circuit := domain.Circuit{Rule: domain.RuleKey{ID: "r", Version: 1}, TenantID: "t", Cause: domain.CauseRateCap,
		Evidence: map[string]any{"fires_last_hour": 4}, OpenedAt: storeNow}
	for range 2 {
		inTx(t, s, func(tx *Tx) error { return tx.OpenCircuit(t.Context(), circuit) })
	}
	inTx(t, s, func(tx *Tx) error {
		open, err := tx.OpenCircuits(t.Context(), "t")
		if err != nil || !open[circuit.Rule] {
			t.Fatalf("open circuits = %v, %v", open, err)
		}
		return nil
	})
	var alerts int
	if err := db.QueryRowContext(t.Context(), "SELECT COUNT(*) FROM notifications WHERE event_type = 'playbook.rule.circuit_opened'").Scan(&alerts); err != nil || alerts != 1 {
		t.Fatalf("%d circuit notifications (%v), want one", alerts, err)
	}
}

func TestForeignReadsSeeNoEpisodeAndTheAuditQueueJoinsTheTransaction(t *testing.T) {
	t.Parallel()
	s := New(storagetest.OpenTemp(t))
	inTx(t, s, func(tx *Tx) error {
		if live, err := tx.HasLiveEpisode(t.Context(), "s1"); err != nil || live {
			t.Fatalf("live episode = %v, %v; want none", live, err)
		}
		if decision, err := tx.Decision(t.Context(), "missing"); err != nil || decision.Ended {
			t.Fatalf("decision = %+v, %v; want an episode that has not ended", decision, err)
		}
		queue := func(_ context.Context, joined *sql.Tx, request domain.AuditRequest) (string, error) {
			if joined != tx.tx {
				t.Fatal("the audit queue did not join the playbook transaction")
			}
			return "sch-" + request.Key, nil
		}
		itemID, err := tx.QueueAudit(t.Context(), queue, domain.AuditRequest{Key: "k"})
		if err != nil || itemID != "sch-k" {
			t.Fatalf("queued audit = %q, %v", itemID, err)
		}
		return nil
	})
}

func TestTheLastDecisionOfAnEndedEpisodeIsRead(t *testing.T) {
	t.Parallel()
	empty, err := lastDecision(nil)
	if err != nil || !empty.Ended || empty.Accepted {
		t.Fatalf("no decision = %+v, %v; want an ended episode without a decision", empty, err)
	}
	views := []episodes.DecisionView{
		{DecisionID: "d1", ValidationStatus: "rejected", Decision: json.RawMessage(`{"diagnosis_code":"x"}`)},
		{DecisionID: "d2", ValidationStatus: "accepted", Decision: json.RawMessage(`{"diagnosis_code":"wear"}`)},
	}
	last, err := lastDecision(views)
	if err != nil || !last.Accepted || last.Decision["diagnosis_code"] != "wear" {
		t.Fatalf("last decision = %+v, %v", last, err)
	}
	if _, err := lastDecision([]episodes.DecisionView{{DecisionID: "d3", Decision: json.RawMessage(`{`)}}); err == nil || !strings.Contains(err.Error(), "decode decision d3") {
		t.Fatalf("a malformed stored decision = %v, want it refused", err)
	}
}
