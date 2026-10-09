package app_test

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/actions/internal/app"
	"github.com/ghassan-ai-projects/agentic-stream/internal/actions/internal/store"
	"github.com/ghassan-ai-projects/agentic-stream/internal/kernel"
)

func TestNewRefusesMissingSafetyDependencies(t *testing.T) {
	t.Parallel()
	db, _ := openActionFixture(t)
	owner := func(context.Context, *sql.Tx, string) error { return nil }
	effector := &recordingEffector{}
	cases := map[string]app.Config{
		"database": {Store: store.New(nil, owner, "epoch"), Effector: effector},
		"owner":    {Store: store.New(db, nil, "epoch"), Effector: effector},
		"effector": {Store: store.New(db, owner, "epoch")},
	}
	for name, cfg := range cases {
		if service, err := app.New(cfg); err == nil || service != nil {
			t.Fatalf("service without %s was constructed", name)
		}
	}
}

func TestNewAppliesDefaultsThatDoNotBypassAuthorization(t *testing.T) {
	t.Parallel()
	db, commandID := openActionFixture(t)
	owner := func(context.Context, *sql.Tx, string) error { return nil }
	service, err := app.New(app.Config{Store: store.New(db, owner, "epoch"), Effector: &recordingEffector{}})
	if err != nil {
		t.Fatal(err)
	}
	if processed, err := service.DispatchOnce(t.Context()); err != nil || !processed {
		t.Fatalf("default-configured dispatch processed=%v err=%v", processed, err)
	}
	var status string
	if err := db.QueryRowContext(t.Context(), "SELECT status FROM commands WHERE command_id = ?", commandID).Scan(&status); err != nil || status != "succeeded" {
		t.Fatalf("status = %q, %v", status, err)
	}
}

func TestDispatchStopsWithoutRuntimeOwnership(t *testing.T) {
	t.Parallel()
	db, commandID := openActionFixture(t)
	effector := &recordingEffector{}
	lost := errors.New("ownership lost")
	service := newServiceWithOwner(t, db, effector, "owner", time.Minute, func(context.Context, *sql.Tx, string) error { return lost })
	processed, err := service.DispatchOnce(t.Context())
	if !errors.Is(err, lost) || processed || effector.calls != 0 {
		t.Fatalf("processed=%v calls=%d err=%v; a dispatcher without ownership must not lease or dispatch", processed, effector.calls, err)
	}
	var status string
	if err := db.QueryRowContext(t.Context(), "SELECT status FROM commands WHERE command_id = ?", commandID).Scan(&status); err != nil || status != "pending" {
		t.Fatalf("status = %q, %v", status, err)
	}
}

type countingObserver struct{ expiries int }

func (o *countingObserver) ObserveLeaseExpiry() { o.expiries++ }

func TestExpiredLeaseIsObserved(t *testing.T) {
	t.Parallel()
	db, commandID := openActionFixture(t)
	claimedAt := kernel.FormatTime(time.Now().UTC().Add(-2 * time.Minute))
	if _, err := db.ExecContext(t.Context(), `UPDATE commands SET status = 'dispatching' WHERE command_id = ?`, commandID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(t.Context(), `UPDATE outbox SET status = 'leased', lease_owner = 'crashed', lease_until = ?, attempt_count = 1 WHERE aggregate_id = ?`, claimedAt, commandID); err != nil {
		t.Fatal(err)
	}
	observer := &countingObserver{}
	owner := func(context.Context, *sql.Tx, string) error { return nil }
	service, err := app.New(app.Config{Store: store.New(db, owner, "epoch"), Effector: &recordingEffector{}, Observer: observer})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.DispatchOnce(t.Context()); err != nil {
		t.Fatal(err)
	}
	if observer.expiries == 0 {
		t.Fatal("an abandoned lease was not observed")
	}
}
