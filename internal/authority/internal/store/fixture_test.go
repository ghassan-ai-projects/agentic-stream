package store

import (
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/authority/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage/storagetest"
)

var (
	testNow = time.Date(2026, 10, 5, 12, 0, 0, 0, time.UTC)
	owner   = domain.Owner{Epoch: "epoch-1", Instance: "instance-1"}
	bootA   = domain.DeviceBoot{DeviceID: "thermal-01", BootID: "boot-A"}
	claim   = domain.TargetClaim{Target: "fan-01", Device: bootA, Owner: owner}
)

func openStore(t *testing.T) (*Store, *storage.DB) {
	t.Helper()
	db := storagetest.OpenTemp(t)

	return New(db), db
}

// work runs fn in a committed priority unit of work and fails the test on error.
func work(t *testing.T, s *Store, fn func(*Tx) error) {
	t.Helper()
	if err := s.InTx(t.Context(), fn); err != nil {
		t.Fatal(err)
	}
}
