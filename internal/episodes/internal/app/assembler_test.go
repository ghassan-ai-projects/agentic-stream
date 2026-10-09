package app_test

import (
	"database/sql"
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/control"
	"github.com/ghassan-ai-projects/agentic-stream/internal/control/controltest"
	"github.com/ghassan-ai-projects/agentic-stream/internal/episodeledger"
	"github.com/ghassan-ai-projects/agentic-stream/internal/episodes/internal/app"
	"github.com/ghassan-ai-projects/agentic-stream/internal/episodes/internal/store"
	"github.com/ghassan-ai-projects/agentic-stream/internal/sources"
	"github.com/ghassan-ai-projects/agentic-stream/internal/spec"
)

func nativeSpec(intents ...spec.Intent) *spec.CompiledSpec {
	return triggeredSpec(spec.Executor{Name: "native", ModelPolicy: "test-policy", PromptVersion: "prompt-v1", Prompt: "Analyze the situation and return a typed decision."}, intents...)
}

func ticketIntent(intentType string) spec.Intent {
	return spec.Intent{Type: intentType, Risk: "R1", ParameterSchema: ticketSchema(), Policy: "automatic", RateLimitPerHour: 2}
}

func assembleInTx(t *testing.T, s *admittedSituation, tenant string) (*app.Request, error) {
	t.Helper()
	var req *app.Request
	err := s.db.WithTx(t.Context(), func(tx *sql.Tx) error {
		var err error
		req, err = s.asm.Assemble(t.Context(), store.Join(tx), s.schedulerItemID, tenant)
		return err
	})
	return req, err
}

func TestAssemblerBindsTheRequestToTheSituationSnapshotAndTheSpec(t *testing.T) {
	t.Parallel()
	s := admitTriggeredSituation(t, nativeSpec(ticketIntent("create_ticket"), ticketIntent("withdraw_maintenance_ticket")), "sit-1")

	req := s.assemble(t)

	if req.EpisodeID == "" || req.SchedulerItemID != s.schedulerItemID || req.SituationID != s.version.SituationID ||
		req.SituationVersion != 1 || req.ExecutorName != "native" || req.TenantID != "default" {
		t.Fatalf("request identity = %+v", req)
	}
	if req.SnapshotSHA256 == "" || req.PromptSHA256 == "" || req.ObjectiveSHA256 == "" || len(req.AdmissionKey) != 32 {
		t.Fatalf("request provenance = snapshot %q prompt %q objective %q admission key %d bytes", req.SnapshotSHA256, req.PromptSHA256, req.ObjectiveSHA256, len(req.AdmissionKey))
	}
	var payload struct {
		AllowedIntentTypes []string       `json:"allowed_intent_types"`
		Delta              map[string]any `json:"delta"`
	}
	if err := json.Unmarshal(req.RequestJSON, &payload); err != nil {
		t.Fatalf("unmarshal request: %v", err)
	}
	if want := []string{"create_ticket", "withdraw_maintenance_ticket"}; !slices.Equal(payload.AllowedIntentTypes, want) {
		t.Fatalf("allowed intent types = %v, want %v", payload.AllowedIntentTypes, want)
	}
	if payload.Delta == nil {
		t.Fatal("request carries no trigger delta")
	}
}

func TestAssemblerCarriesTheWatchConfidenceFloor(t *testing.T) {
	t.Parallel()
	float := func(value float64) *float64 { return &value }
	tests := []struct {
		name  string
		floor *float64
		want  float64
	}{
		{"default", nil, 0.5},
		{"custom", float(0.7), 0.7},
		{"opt out", float(0), 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			compiled := nativeSpec(ticketIntent("create_ticket"))
			compiled.Actions.WatchConfidenceFloor = tc.floor
			s := admitTriggeredSituation(t, compiled, "sit-1")

			var payload struct {
				WatchConfidenceFloor float64 `json:"watch_confidence_floor"`
			}
			if err := json.Unmarshal(s.assemble(t).RequestJSON, &payload); err != nil {
				t.Fatalf("unmarshal request: %v", err)
			}
			if payload.WatchConfidenceFloor != tc.want {
				t.Fatalf("watch confidence floor = %v, want %v", payload.WatchConfidenceFloor, tc.want)
			}
		})
	}
}

func TestAssemblingTheSameItemTwiceBindsTheSameSnapshot(t *testing.T) {
	t.Parallel()
	s := admitTriggeredSituation(t, nativeSpec(ticketIntent("create_ticket")), "sit-1")

	first, second := s.assemble(t), s.assemble(t)

	if first.SnapshotSHA256 != second.SnapshotSHA256 || first.PromptSHA256 != second.PromptSHA256 {
		t.Fatalf("snapshot %s vs %s, prompt %s vs %s", first.SnapshotSHA256, second.SnapshotSHA256, first.PromptSHA256, second.PromptSHA256)
	}
}

func TestAssemblerRefusesAnItemOfAnotherTenant(t *testing.T) {
	t.Parallel()
	s := admitTriggeredSituation(t, nativeSpec(ticketIntent("create_ticket")), "sit-1")

	req, err := assembleInTx(t, s, "other-tenant")

	if err == nil || !strings.Contains(err.Error(), "tenant mismatch") || req != nil {
		t.Fatalf("assemble = %+v, %v; want a tenant mismatch", req, err)
	}
}

func TestAssemblerRefusesAnItemWhoseEvidenceIsBroken(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		statement string
		want      string
	}{
		{"unknown scheduler item", "UPDATE scheduler_items SET scheduler_item_id = 'moved'", "load scheduler item"},
		{"missing trigger evaluation", "UPDATE scheduler_items SET trigger_id = 'missing'", "load evaluation"},
		{"missing snapshot", "UPDATE scheduler_items SET situation_version = 99", "load snapshot"},
		{"invalid snapshot trace context", "UPDATE situation_versions SET traceparent = 'invalid'", "validate situation trace context"},
		{"undecodable delta", "UPDATE trigger_evaluations SET delta_json = X'7B'", "unmarshal delta"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			s := admitTriggeredSituation(t, nativeSpec(ticketIntent("create_ticket")), "sit-1")
			execWithoutForeignKeys(t, s.db, stmt(tc.statement))

			if _, err := assembleInTx(t, s, "default"); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %v, want one containing %q", err, tc.want)
			}
		})
	}
}

func TestPersistAdmitsTheEpisodeAndTheSchedulerItem(t *testing.T) {
	t.Parallel()
	s := admitTriggeredSituation(t, nativeSpec(ticketIntent("create_ticket")), "sit-1")

	s.assembleAndPersist(t)

	if got := scalar[string](t, s.db, "SELECT lifecycle_status FROM episodes WHERE scheduler_item_id = ?", s.schedulerItemID); got != "admitted" {
		t.Fatalf("episode lifecycle = %q, want admitted", got)
	}
	if got := scalar[string](t, s.db, "SELECT status FROM scheduler_items WHERE scheduler_item_id = ?", s.schedulerItemID); got != "admitted" {
		t.Fatalf("scheduler item status = %q, want admitted", got)
	}
	if got := scalar[int](t, s.db, "SELECT length(prompt_sha256) + length(objective_sha256) FROM episodes WHERE scheduler_item_id = ?", s.schedulerItemID); got != 64 {
		t.Fatalf("stored provenance digest bytes = %d, want 64", got)
	}
}

func TestPersistRefusesARequestWithoutDecodableProvenance(t *testing.T) {
	t.Parallel()
	s := admitTriggeredSituation(t, nativeSpec(ticketIntent("create_ticket")), "sit-1")
	req := s.assemble(t)
	req.PromptSHA256 = "not-a-digest"

	err := s.db.WithTx(t.Context(), func(tx *sql.Tx) error { return s.asm.Persist(t.Context(), store.Join(tx), req, s.base) })

	if err == nil || !strings.Contains(err.Error(), "decode prompt digest") {
		t.Fatalf("error = %v, want decode prompt digest", err)
	}
	if got := scalar[int](t, s.db, "SELECT COUNT(*) FROM episodes"); got != 0 {
		t.Fatalf("episodes after refused persist = %d, want 0", got)
	}
}

func TestPersistRefusesAnItemThatIsNoLongerPending(t *testing.T) {
	t.Parallel()
	s := admitTriggeredSituation(t, nativeSpec(ticketIntent("create_ticket")), "sit-1")
	req := s.assemble(t)

	err := s.db.WithTx(t.Context(), func(tx *sql.Tx) error {
		if err := s.asm.Persist(t.Context(), store.Join(tx), req, s.base); err != nil {
			t.Fatalf("first persist: %v", err)
		}
		return s.asm.Persist(t.Context(), store.Join(tx), req, s.base)
	})

	if err == nil || errors.Is(err, episodeledger.ErrLiveEpisodeConflict) {
		t.Fatalf("second persist error = %v, want a refusal that is not a live-episode conflict", err)
	}
}

func TestPersistReservesTheEpisodeCostBudgetWhenCostControlIsOn(t *testing.T) {
	t.Parallel()
	compiled := nativeSpec(ticketIntent("create_ticket"))
	compiled.Cognition.Executor.Budget.CostMicrounits = 40
	s := admitTriggeredSituation(t, compiled, "sit-1")
	asm := app.NewAssembler(compiled, sources.Deterministic()).WithCostLedger(&control.CostLedger{})
	req := s.assemble(t)

	if err := s.db.WithTx(t.Context(), func(tx *sql.Tx) error { return asm.Persist(t.Context(), store.Join(tx), req, s.base) }); err != nil {
		t.Fatal(err)
	}

	if got := scalar[int](t, s.db, "SELECT reserved_micro FROM cost_reservations WHERE episode_id = ?", req.EpisodeID); got != 40 {
		t.Fatalf("reserved microunits = %d, want the 40 admitted", got)
	}
}

func TestPersistRefusesAnEpisodeThatExceedsTheCostCeiling(t *testing.T) {
	t.Parallel()
	compiled := nativeSpec(ticketIntent("create_ticket"))
	compiled.Cognition.Executor.Budget.CostMicrounits = 40
	s := admitTriggeredSituation(t, compiled, "sit-1")
	if err := s.db.WithTx(t.Context(), func(tx *sql.Tx) error {
		return controltest.SetCostLimit(t.Context(), tx, "global", "", 10, false, "2026-08-14T12:00:00Z")
	}); err != nil {
		t.Fatal(err)
	}
	asm := app.NewAssembler(compiled, sources.Deterministic()).WithCostLedger(&control.CostLedger{})
	req := s.assemble(t)

	err := s.db.WithTx(t.Context(), func(tx *sql.Tx) error { return asm.Persist(t.Context(), store.Join(tx), req, s.base) })

	if !errors.Is(err, control.ErrCostReservationRejected) {
		t.Fatalf("persist error = %v, want ErrCostReservationRejected", err)
	}
	if got := scalar[int](t, s.db, "SELECT COUNT(*) FROM episodes"); got != 0 {
		t.Fatalf("episodes after a refused reservation = %d, want 0", got)
	}
}
