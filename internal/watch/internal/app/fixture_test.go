package app_test

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/actionport"
	"github.com/ghassan-ai-projects/agentic-stream/internal/kernel"
	"github.com/ghassan-ai-projects/agentic-stream/internal/sources"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage/storagetest"
	"github.com/ghassan-ai-projects/agentic-stream/internal/watch/internal/app"
	"github.com/ghassan-ai-projects/agentic-stream/internal/watch/internal/store"
)

var fixtureNow = time.Date(2026, 10, 6, 12, 0, 0, 0, time.UTC)

func allowOwner(context.Context, *sql.Tx, string) error { return nil }

func newService(t *testing.T, db *storage.DB) *app.Service {
	t.Helper()
	return newServiceAt(t, db, sources.NewVirtual(fixtureNow))
}

func newServiceAt(t *testing.T, db *storage.DB, clock sources.Clock) *app.Service {
	t.Helper()
	return newServiceOwnedBy(t, db, clock, allowOwner)
}

func newServiceOwnedBy(t *testing.T, db *storage.DB, clock sources.Clock, owner storage.OwnerCheck) *app.Service {
	t.Helper()
	service, err := app.New(app.Config{Store: store.New(db, owner, "epoch"), Clock: clock})
	if err != nil {
		t.Fatalf("new watch service: %v", err)
	}
	return service
}

func watchCommand(id, expression, target string, maxFires int, expiresAt time.Time) actionport.Command {
	return actionport.Command{CommandID: id, TenantID: "tenant-1", EffectorRoute: "install_watch_condition",
		Payload: map[string]any{"expression": expression, "target": target, "expires_at": kernel.FormatTime(expiresAt),
			"situation_id": "sit-1", "situation_version": 1, "max_fires": maxFires}}
}

func installable(id string) actionport.Command {
	return watchCommand(id, "features.temperature > 90", "motor-1", 2, fixtureNow.Add(24*time.Hour))
}

func install(t *testing.T, service *app.Service, command actionport.Command) {
	t.Helper()
	if _, err := service.Dispatch(t.Context(), command); err != nil {
		t.Fatalf("install %s: %v", command.CommandID, err)
	}
}

type watchState struct {
	Status    string
	Remaining int
	Fires     int
}

func readWatch(t *testing.T, db *storage.DB, watchID string) watchState {
	t.Helper()
	var state watchState
	err := db.QueryRowContext(t.Context(), `SELECT status, remaining_fires, (SELECT COUNT(*) FROM watch_fires WHERE watch_id = w.watch_id)
		FROM watch_conditions w WHERE watch_id = ?`, watchID).Scan(&state.Status, &state.Remaining, &state.Fires)
	if err != nil {
		t.Fatalf("read watch %s: %v", watchID, err)
	}
	return state
}

func countWatches(t *testing.T, db *storage.DB) int {
	t.Helper()
	var n int
	if err := db.QueryRowContext(t.Context(), "SELECT COUNT(*) FROM watch_conditions").Scan(&n); err != nil {
		t.Fatalf("count watches: %v", err)
	}
	return n
}

func openDB(t *testing.T) *storage.DB {
	t.Helper()
	return storagetest.OpenTemp(t)
}
