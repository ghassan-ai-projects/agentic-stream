package actions_test

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/actionport"
	"github.com/ghassan-ai-projects/agentic-stream/internal/actions"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
	"github.com/ghassan-ai-projects/agentic-stream/internal/telemetry"
)

type noopEffector struct{}

func (noopEffector) DispatchAuthorized(ctx context.Context, command actionport.Command, authorization actionport.Authorization) (actionport.Effect, error) {
	if err := authorization.Check(ctx); err != nil {
		return actionport.Effect{}, err
	}
	return actionport.Effect{}, nil
}

func (noopEffector) Dispatch(context.Context, actionport.Command) (actionport.Effect, error) {
	return actionport.Effect{}, nil
}

func openDB(t *testing.T) *storage.DB {
	t.Helper()
	db, err := storage.Open(t.Context(), filepath.Join(t.TempDir(), "actions.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func owned(context.Context, *sql.Tx, string) error { return nil }

func TestNewRefusesEveryMissingSafetyDependency(t *testing.T) {
	t.Parallel()
	db := openDB(t)
	complete := actions.Config{DB: db, Effector: noopEffector{}, RuntimeOwner: owned, Epoch: "epoch"}
	cases := map[string]func(*actions.Config){
		"database": func(c *actions.Config) { c.DB = nil },
		"effector": func(c *actions.Config) { c.Effector = nil },
		"owner":    func(c *actions.Config) { c.RuntimeOwner = nil },
	}
	for name, remove := range cases {
		cfg := complete
		remove(&cfg)
		if service, err := actions.New(cfg); err == nil || service != nil {
			t.Fatalf("service without %s was constructed", name)
		}
	}
	if _, err := actions.New(complete); err != nil {
		t.Fatalf("complete configuration refused: %v", err)
	}
}

func TestServiceDispatchesNothingFromAnEmptyOutbox(t *testing.T) {
	t.Parallel()
	service, err := actions.New(actions.Config{DB: openDB(t), Effector: noopEffector{}, RuntimeOwner: owned, Epoch: "epoch",
		Telemetry: &telemetry.Runtime{}})
	if err != nil {
		t.Fatal(err)
	}
	if processed, err := service.DispatchOnce(t.Context()); err != nil || processed {
		t.Fatalf("empty outbox processed=%v err=%v", processed, err)
	}
}

func TestCountUnresolvedOutcomesJoinsTheCallersTransaction(t *testing.T) {
	t.Parallel()
	db := openDB(t)
	if err := db.WithTx(t.Context(), func(tx *sql.Tx) error {
		if got, err := actions.CountUnresolvedOutcomes(t.Context(), tx, nil); err != nil || got != 0 {
			t.Fatalf("no ids: %d, %v", got, err)
		}
		if got, err := actions.CountUnresolvedOutcomes(t.Context(), tx, []string{"cmd-missing"}); err != nil || got != 0 {
			t.Fatalf("missing command: %d, %v", got, err)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}
