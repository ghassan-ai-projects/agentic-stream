package app_test

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/actionport"
	"github.com/ghassan-ai-projects/agentic-stream/internal/interlock"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
	"github.com/ghassan-ai-projects/agentic-stream/internal/watch/internal/app"
	"github.com/ghassan-ai-projects/agentic-stream/internal/watch/internal/store"
)

func openDB(t *testing.T) *storage.DB {
	t.Helper()
	db, err := storage.Open(t.Context(), filepath.Join(t.TempDir(), "watch.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func installable(id string) actionport.Command {
	return actionport.Command{CommandID: id, TenantID: "tenant-1", EffectorRoute: "install_watch_condition",
		Payload: map[string]any{"expression": "features.temperature > 90", "target": "motor-1", "expires_at": "2099-01-01T00:00:00Z",
			"situation_id": "sit-1", "situation_version": 1, "max_fires": 2}}
}

func TestNewRefusesMissingSafetyDependencies(t *testing.T) {
	t.Parallel()
	db := openDB(t)
	owner := func(context.Context, *sql.Tx, string) error { return nil }
	for name, s := range map[string]store.Store{
		"database": store.New(nil, owner, "epoch", interlock.DurableReader{}), "owner": store.New(db, nil, "epoch", interlock.DurableReader{}),
		"interlock": store.New(db, owner, "epoch", nil),
	} {
		if service, err := app.New(app.Config{Store: s}); err == nil || service != nil {
			t.Fatalf("service without %s was constructed", name)
		}
	}
}

func TestInstallRefusesWithoutRuntimeOwnership(t *testing.T) {
	t.Parallel()
	db := openDB(t)
	lost := errors.New("ownership lost")
	service, err := app.New(app.Config{Store: store.New(db, func(context.Context, *sql.Tx, string) error { return lost }, "epoch", interlock.DurableReader{})})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Dispatch(t.Context(), installable("cmd-1")); !errors.Is(err, lost) {
		t.Fatalf("err = %v, want the ownership error", err)
	}
	if err := service.Expire(t.Context()); !errors.Is(err, lost) {
		t.Fatalf("expire err = %v, want the ownership error", err)
	}
}

func TestInstallRefusesWhenInterlockTrips(t *testing.T) {
	t.Parallel()
	db := openDB(t)
	if err := db.WithTx(t.Context(), func(tx *sql.Tx) error {
		_, err := interlock.Trip(t.Context(), tx, "stop", time.Now().UTC().Format(time.RFC3339Nano))
		return err
	}); err != nil {
		t.Fatal(err)
	}
	service := newService(t, db, nil)
	if _, err := service.Dispatch(t.Context(), installable("cmd-1")); err == nil || !strings.Contains(err.Error(), "interlock") {
		t.Fatalf("err = %v, want an interlock refusal", err)
	}
}

func TestInstallRefusesUnsupportedRouteAndConflictingReinstall(t *testing.T) {
	t.Parallel()
	service := newService(t, openDB(t), nil)
	other := installable("cmd-1")
	other.EffectorRoute = "other"
	if _, err := service.Dispatch(t.Context(), other); err == nil || !strings.Contains(err.Error(), "does not support route") {
		t.Fatalf("route err = %v", err)
	}
	command := installable("cmd-1")
	if effect, err := service.Dispatch(t.Context(), command); err != nil || effect.ProviderResult["watch_id"] != "cmd-1" {
		t.Fatalf("install effect = %+v err=%v", effect, err)
	}
	command.Payload["max_fires"] = 5
	if _, err := service.Dispatch(t.Context(), command); err == nil || !strings.Contains(err.Error(), "idempotency conflict") {
		t.Fatalf("conflict err = %v", err)
	}
}

func TestAuthorizedDispatchRunsTheFinalCheckFirst(t *testing.T) {
	t.Parallel()
	service := newService(t, openDB(t), nil)
	if _, err := service.DispatchAuthorized(t.Context(), installable("cmd-1"), actionport.Authorization{}); err == nil {
		t.Fatal("authorized dispatch without a check was accepted")
	}
	denied := errors.New("interlock tripped")
	if _, err := service.DispatchAuthorized(t.Context(), installable("cmd-1"), actionport.Authorization{Check: func(context.Context) error { return denied }}); !errors.Is(err, denied) {
		t.Fatalf("err = %v, want the check error", err)
	}
	checked := false
	allow := actionport.Authorization{Check: func(context.Context) error { checked = true; return nil }}
	if _, err := service.DispatchAuthorized(t.Context(), installable("cmd-1"), allow); err != nil || !checked {
		t.Fatalf("checked=%v err=%v", checked, err)
	}
}

func TestFireRequiresIdentity(t *testing.T) {
	t.Parallel()
	service := newService(t, openDB(t), nil)
	if _, err := service.FireEvent(t.Context(), "", "motor-1", nil); err == nil {
		t.Fatal("event without an ID fired")
	}
	if _, err := service.FireEvent(t.Context(), "evt-1", "", nil); err == nil {
		t.Fatal("event without a target fired")
	}
	if _, err := service.Fire(t.Context(), "", "evt-1", "sit-1", "motor-1", nil); err == nil {
		t.Fatal("fire without a watch ID was accepted")
	}
}

func TestExpireWaitHonorsCancellation(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if err := app.AwaitRetry(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("retry wait = %v", err)
	}
}
