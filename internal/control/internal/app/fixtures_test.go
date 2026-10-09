package app

import (
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/control/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/control/internal/store"
	"github.com/ghassan-ai-projects/agentic-stream/internal/sources"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage/storagetest"
)

const costTime = "2026-01-01T00:00:00Z"

var base = time.Date(2026, 8, 12, 12, 0, 0, 0, time.UTC)

func openStore(t *testing.T) store.Store {
	t.Helper()
	persistence, _ := openStoreWithDB(t)
	return persistence
}

func openStoreWithDB(t *testing.T) (store.Store, *storage.DB) {
	t.Helper()
	db := storagetest.OpenTemp(t)
	return store.New(db), db
}

func owner(persistence store.Store, instance string, clock *sources.Virtual) *Owner {
	return &Owner{Store: persistence, Instance: instance, Lease: time.Minute, Now: clock.Now}
}

func epochsAt(persistence store.Store, clock *sources.Virtual) *Epochs {
	return &Epochs{Store: persistence, Now: clock.Now}
}

func tenantLimit(t *testing.T, persistence store.Store, tenant string, maxMicro uint64, kill bool) {
	t.Helper()
	setLimit(t, persistence, domain.TenantScope(tenant), tenant, maxMicro, kill)
}

func setLimit(t *testing.T, persistence store.Store, scope, tenant string, maxMicro uint64, kill bool) {
	t.Helper()
	err := persistence.WithTx(t.Context(), func(tx *store.Tx) error {
		return SetLimit(t.Context(), tx, scope, tenant, maxMicro, kill, costTime)
	})
	if err != nil {
		t.Fatalf("set %s limit: %v", scope, err)
	}
}

func reserve(t *testing.T, persistence store.Store, episode, tenant string, amount uint64) error {
	t.Helper()
	return persistence.WithTx(t.Context(), func(tx *store.Tx) error {
		return Reserve(t.Context(), tx, episode, tenant, amount, costTime)
	})
}

func settle(t *testing.T, persistence store.Store, episode string, actual uint64) error {
	t.Helper()
	return persistence.WithTx(t.Context(), func(tx *store.Tx) error {
		return Settle(t.Context(), tx, episode, actual, costTime)
	})
}
