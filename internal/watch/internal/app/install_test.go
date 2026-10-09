package app_test

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/sources"

	"github.com/ghassan-ai-projects/agentic-stream/internal/actionport"
	"github.com/ghassan-ai-projects/agentic-stream/internal/interlock"
	"github.com/ghassan-ai-projects/agentic-stream/internal/watch/internal/app"
	"github.com/ghassan-ai-projects/agentic-stream/internal/watch/internal/store"
)

func TestNewRefusesMissingSafetyDependencies(t *testing.T) {
	t.Parallel()
	db := openDB(t)
	cases := map[string]store.Store{"database": store.New(nil, allowOwner, "epoch"), "owner": store.New(db, nil, "epoch")}
	for name, s := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if service, err := app.New(app.Config{Store: s}); err == nil || service != nil {
				t.Fatalf("service without %s was constructed", name)
			}
		})
	}
}

func TestAnInstalledWatchIsActiveWithItsFullAllowance(t *testing.T) {
	t.Parallel()
	db := openDB(t)
	effect, err := newService(t, db).Dispatch(t.Context(), installable("cmd-1"))
	if err != nil || effect.ProviderResult["watch_id"] != "cmd-1" {
		t.Fatalf("effect = %+v, err = %v; want the installed watch ID", effect, err)
	}
	if got, want := readWatch(t, db, "cmd-1"), (watchState{Status: "active", Remaining: 2}); got != want {
		t.Fatalf("watch = %+v, want %+v", got, want)
	}
}

func TestAWatchIsIdentifiedByTheIdempotencyKeyBeforeTheCommandID(t *testing.T) {
	t.Parallel()
	db := openDB(t)
	command := installable("cmd-1")
	command.IdempotencyKey = "key-1"
	effect, err := newService(t, db).Dispatch(t.Context(), command)
	if err != nil || effect.ProviderResult["watch_id"] != "key-1" {
		t.Fatalf("effect = %+v, err = %v; want the idempotency key as the watch ID", effect, err)
	}
	if n := countWatches(t, db); n != 1 {
		t.Fatalf("watches = %d, want 1", n)
	}
}

func TestReinstallingTheSameWatchIsIdempotentAndAConflictingOneIsRefused(t *testing.T) {
	t.Parallel()
	db := openDB(t)
	service := newService(t, db)
	install(t, service, installable("cmd-1"))
	install(t, service, installable("cmd-1"))
	if n := countWatches(t, db); n != 1 {
		t.Fatalf("watches = %d, want the repeated install to leave one", n)
	}
	conflicting := installable("cmd-1")
	conflicting.Payload["max_fires"] = 5
	if _, err := service.Dispatch(t.Context(), conflicting); err == nil || !strings.Contains(err.Error(), "idempotency conflict") {
		t.Fatalf("conflict err = %v, want an idempotency conflict", err)
	}
	if got := readWatch(t, db, "cmd-1"); got.Remaining != 2 {
		t.Fatalf("watch = %+v, want the original allowance kept", got)
	}
}

func TestOnlyTheInstallRouteIsSupported(t *testing.T) {
	t.Parallel()
	db := openDB(t)
	other := installable("cmd-1")
	other.EffectorRoute = "other"
	if _, err := newService(t, db).Dispatch(t.Context(), other); err == nil || !strings.Contains(err.Error(), "does not support route") {
		t.Fatalf("route err = %v, want the unsupported route refusal", err)
	}
	if n := countWatches(t, db); n != 0 {
		t.Fatalf("watches = %d, want none", n)
	}
}

func TestInstallRefusesWithoutRuntimeOwnership(t *testing.T) {
	t.Parallel()
	db := openDB(t)
	lost := errors.New("ownership lost")
	service := newServiceOwnedBy(t, db, sources.NewVirtual(fixtureNow), func(context.Context, *sql.Tx, string) error { return lost })
	if _, err := service.Dispatch(t.Context(), installable("cmd-1")); !errors.Is(err, lost) {
		t.Fatalf("install err = %v, want the ownership error", err)
	}
	if err := service.Expire(t.Context()); !errors.Is(err, lost) {
		t.Fatalf("expire err = %v, want the ownership error", err)
	}
	if n := countWatches(t, db); n != 0 {
		t.Fatalf("watches = %d, want none", n)
	}
}

func TestATrippedInterlockRefusesBothInstallingAndFiring(t *testing.T) {
	t.Parallel()
	db := openDB(t)
	service := newService(t, db)
	install(t, service, installable("cmd-1"))
	if err := db.WithTx(t.Context(), func(tx *sql.Tx) error {
		_, err := interlock.TripIn(t.Context(), tx, "stop", fixtureNow)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Dispatch(t.Context(), installable("cmd-2")); err == nil || !strings.Contains(err.Error(), "interlock") {
		t.Fatalf("install err = %v, want an interlock refusal", err)
	}
	if _, err := service.FireEvent(t.Context(), "evt-1", "motor-1", map[string]any{"temperature": 95}); err == nil || !strings.Contains(err.Error(), "interlock") {
		t.Fatalf("fire err = %v, want an interlock refusal", err)
	}
	if n, state := countWatches(t, db), readWatch(t, db, "cmd-1"); n != 1 || state.Fires != 0 || state.Remaining != 2 {
		t.Fatalf("watches = %d, first watch = %+v; want no second watch and no fire", n, state)
	}
}

func TestAuthorizedDispatchRefusesWithoutAPassingFinalCheckAndInstallsNothing(t *testing.T) {
	t.Parallel()
	denied := errors.New("interlock tripped")
	cases := []struct {
		name          string
		authorization actionport.Authorization
		check         func(error) bool
	}{
		{"no check supplied", actionport.Authorization{}, func(err error) bool { return err != nil && strings.Contains(err.Error(), "authorization is required") }},
		{"check denies", actionport.Authorization{Check: func(context.Context) error { return denied }}, func(err error) bool { return errors.Is(err, denied) }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			db := openDB(t)
			if _, err := newService(t, db).DispatchAuthorized(t.Context(), installable("cmd-1"), tc.authorization); !tc.check(err) {
				t.Fatalf("err = %v", err)
			}
			if n := countWatches(t, db); n != 0 {
				t.Fatalf("watches = %d, want none", n)
			}
		})
	}
}

func TestAuthorizedDispatchRunsTheFinalCheckOnceThenInstalls(t *testing.T) {
	t.Parallel()
	db := openDB(t)
	checked := 0
	allow := actionport.Authorization{Check: func(context.Context) error { checked++; return nil }}
	if _, err := newService(t, db).DispatchAuthorized(t.Context(), installable("cmd-1"), allow); err != nil || checked != 1 {
		t.Fatalf("checked = %d, err = %v; want one passing check and an install", checked, err)
	}
	if got := readWatch(t, db, "cmd-1"); got.Status != "active" {
		t.Fatalf("watch = %+v, want active", got)
	}
}
