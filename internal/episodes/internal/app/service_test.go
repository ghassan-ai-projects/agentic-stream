package app_test

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"testing"

	runtimecontrol "github.com/ghassan-ai-projects/agentic-stream/internal/control"
	"github.com/ghassan-ai-projects/agentic-stream/internal/episodes/internal/app"
	"github.com/ghassan-ai-projects/agentic-stream/internal/episodes/internal/store"
	"github.com/ghassan-ai-projects/agentic-stream/internal/executor/fixture"
	"github.com/ghassan-ai-projects/agentic-stream/internal/sources"
	"github.com/ghassan-ai-projects/agentic-stream/internal/spec"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage/storagetest"
)

func allowEpoch(context.Context, *sql.Tx, string) error { return nil }

func TestNewRefusesAConfigurationThatCannotDispatchSafely(t *testing.T) {
	t.Parallel()
	db := storagetest.OpenTemp(t)
	compiled := &spec.CompiledSpec{}
	owner := func(context.Context, *sql.Tx, string) error { return nil }
	tests := []struct {
		name string
		cfg  app.Config
		want string
	}{
		{"no spec", app.Config{}, "episode spec is required"},
		{"no database", app.Config{Spec: compiled, Execution: &app.ExecutionConfig{Executor: fixture.New(), DecisionEpoch: allowEpoch}}, "database, executor and decision epoch check"},
		{"no executor", app.Config{Spec: compiled, Execution: &app.ExecutionConfig{Episodes: store.New(db), DecisionEpoch: allowEpoch}}, "database, executor and decision epoch check"},
		{"no epoch check", app.Config{Spec: compiled, Execution: &app.ExecutionConfig{Episodes: store.New(db), Executor: fixture.New()}}, "database, executor and decision epoch check"},
		{"owner epoch without a runtime owner check", app.Config{Spec: compiled, Execution: &app.ExecutionConfig{Episodes: store.New(db), Executor: fixture.New(), DecisionEpoch: allowEpoch, OwnerEpoch: "epoch"}}, "runtime owner check is required"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			service, err := app.New(tc.cfg)
			if err == nil || !strings.Contains(err.Error(), tc.want) || service != nil {
				t.Fatalf("New = (%v, %v), want no service and an error containing %q", service, err, tc.want)
			}
		})
	}
	fenced := store.New(db).Fenced(owner)
	accepted := app.Config{Spec: compiled, Execution: &app.ExecutionConfig{Episodes: fenced, Executor: fixture.New(), DecisionEpoch: allowEpoch, OwnerEpoch: "epoch"}}
	if service, err := app.New(accepted); err != nil || service == nil {
		t.Fatalf("a fenced owner epoch configuration was refused: %v", err)
	}
}

func TestAssemblyOnlyServiceRefusesToRunEpisodesBeforeTouchingState(t *testing.T) {
	t.Parallel()
	service, err := app.New(app.Config{Spec: &spec.CompiledSpec{}})
	if err != nil {
		t.Fatal(err)
	}

	ran, err := service.RunOnce(t.Context(), episodeTenant)

	if err == nil || !strings.Contains(err.Error(), "assembly only") || ran {
		t.Fatalf("RunOnce = (%v, %v), want a refusal naming assembly-only configuration", ran, err)
	}
}

func TestServiceRunsAnAdmittedEpisodeThroughItsConfiguredRunner(t *testing.T) {
	t.Parallel()
	db := storagetest.OpenTemp(t)
	seedEpisode(t, db, "epi-service")
	reserveCost(t, db, "epi-service", 10)
	service, err := app.New(app.Config{
		Spec: &spec.CompiledSpec{}, IDGenerator: sources.Deterministic(), CostControl: &runtimecontrol.CostLedger{},
		Execution: &app.ExecutionConfig{Episodes: store.New(db), Executor: fixture.New(), DecisionEpoch: allowEpoch},
	})
	if err != nil {
		t.Fatal(err)
	}

	ran, err := service.RunOnce(t.Context(), episodeTenant)

	if err != nil || !ran || lifecycleOf(t, db, "epi-service") != "concluded" {
		t.Fatalf("RunOnce ran=%v err=%v lifecycle=%q, want true nil concluded", ran, err, lifecycleOf(t, db, "epi-service"))
	}
}

func TestDecisionsReadsEveryDecisionOfAnEpisodeInOrder(t *testing.T) {
	t.Parallel()
	db := storagetest.OpenTemp(t)
	seedEpisode(t, db, "epi-decisions")
	mustRunOnce(t, permissiveRunner(db, fixture.New()))

	views, err := app.Decisions(t.Context(), store.New(db), "epi-decisions")

	if err != nil || len(views) != 1 {
		t.Fatalf("Decisions = %v, %v; want one decision", views, err)
	}
	if views[0].ValidationStatus != "accepted" || views[0].SituationVersion != 1 || views[0].Fence != 1 || views[0].Ordinal != 1 || !strings.HasPrefix(views[0].DecisionSHA256, "sha256:") {
		t.Fatalf("decision view = %+v", views[0])
	}
}

func TestDecisionsNamesTheEpisodeWhenTheReadFails(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	db := storagetest.OpenTemp(t)

	_, err := app.Decisions(ctx, store.New(db), "epi-missing")

	if err == nil || !strings.Contains(err.Error(), "decisions of episode epi-missing") {
		t.Fatalf("error = %v, want one naming the episode", err)
	}
}

func TestServiceAssemblesAndPersistsOnTheCallersTransaction(t *testing.T) {
	t.Parallel()
	compiled := nativeSpec(ticketIntent("create_ticket"))
	s := admitTriggeredSituation(t, compiled, "sit-service")
	service, err := app.New(app.Config{Spec: compiled, IDGenerator: sources.Deterministic()})
	if err != nil {
		t.Fatal(err)
	}
	rollback := errors.New("caller changed its mind")

	err = s.db.WithTx(t.Context(), func(tx *sql.Tx) error {
		req, err := service.Assemble(t.Context(), store.Join(tx), s.schedulerItemID, "default")
		if err != nil {
			t.Fatalf("assemble: %v", err)
		}
		if err := service.Persist(t.Context(), store.Join(tx), req, s.base); err != nil {
			t.Fatalf("persist: %v", err)
		}
		return rollback
	})

	if !errors.Is(err, rollback) {
		t.Fatalf("transaction error = %v, want the caller's rollback", err)
	}
	if got := scalar[int](t, s.db, "SELECT COUNT(*) FROM episodes"); got != 0 {
		t.Fatalf("episodes after the caller rolled back = %d, want 0", got)
	}
	if got := scalar[string](t, s.db, "SELECT status FROM scheduler_items WHERE scheduler_item_id = ?", s.schedulerItemID); got != "pending" {
		t.Fatalf("scheduler item status after rollback = %q, want pending", got)
	}
}
