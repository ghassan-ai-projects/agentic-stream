package app_test

import (
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/actions/internal/app"
	"github.com/ghassan-ai-projects/agentic-stream/internal/actions/internal/store"
)

func TestNewRefusesMissingSafetyDependencies(t *testing.T) {
	t.Parallel()
	db, _ := openActionFixture(t)
	effector := succeeds()
	cases := map[string]app.Config{
		"database": {Store: store.New(nil, allowOwner, "epoch"), Effector: effector},
		"owner":    {Store: store.New(db, nil, "epoch"), Effector: effector},
		"effector": {Store: store.New(db, allowOwner, "epoch")},
	}
	for name, cfg := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if service, err := app.New(cfg); err == nil || service != nil {
				t.Fatalf("service without %s was constructed", name)
			}
		})
	}
}

func TestNewAppliesDefaultsThatDoNotBypassAuthorization(t *testing.T) {
	t.Parallel()
	db, commandID := openFixture(t, fixtureSpec{now: time.Now().UTC()})
	service, err := app.New(app.Config{Store: store.New(db, allowOwner, "epoch"), Effector: succeeds()})
	if err != nil {
		t.Fatal(err)
	}
	if processed, err := service.DispatchOnce(t.Context()); err != nil || !processed {
		t.Fatalf("default-configured dispatch processed=%v err=%v", processed, err)
	}
	if got := readLedger(t, db, commandID); got.Command != "succeeded" {
		t.Fatalf("ledger = %+v, want a succeeded command under the default clock, identities and lease", got)
	}
}
