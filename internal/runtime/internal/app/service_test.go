package app

import (
	"context"
	"errors"
	"testing"
	"time"

	runtimecontrol "github.com/ghassan-ai-projects/agentic-stream/internal/control"
	"github.com/ghassan-ai-projects/agentic-stream/internal/evidence"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage/storagetest"
)

func newTestService(t *testing.T, lease time.Duration) (*Service, *storage.DB) {
	t.Helper()
	db := storagetest.OpenTemp(t)
	owner := &runtimecontrol.RuntimeOwner{DB: db, InstanceID: "instance-service", Lease: lease}
	ledger, err := evidence.New(evidence.Config{Ledger: &evidence.LedgerConfig{OwnerCheck: owner.Assert, DB: db, LeaseOwner: "instance-service", RuntimeEpoch: "epoch-service", Lease: lease}})
	if err != nil {
		t.Fatal(err)
	}
	service, err := NewService(owner, ledger, "epoch-service")
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	return service, db
}

func TestServiceIsReadyOnlyBetweenStartAndClose(t *testing.T) {
	t.Parallel()
	service, _ := newTestService(t, time.Minute)
	if err := service.Ready(); err == nil {
		t.Fatal("service was ready before start")
	}
	if _, err := service.Start(t.Context()); err != nil {
		t.Fatalf("start service: %v", err)
	}
	if err := service.Ready(); err != nil {
		t.Fatalf("service not ready after start: %v", err)
	}
	if err := service.Close(t.Context()); err != nil {
		t.Fatalf("close service: %v", err)
	}
	if err := service.Ready(); err == nil {
		t.Fatal("service remained ready after close")
	}
}

func TestServiceRequiresItsConfigurationAndToleratesAnAbsentInstance(t *testing.T) {
	t.Parallel()
	if _, err := NewService(nil, nil, ""); err == nil {
		t.Fatal("missing service configuration accepted")
	}
	var service *Service
	if _, err := service.Start(t.Context()); err == nil {
		t.Fatal("nil service started")
	}
	if service.Ready() == nil || service.Close(t.Context()) != nil {
		t.Fatal("a nil service must report not ready and close without error")
	}
}

func TestTheHeartbeatClearsReadinessWhenTheOwnerLeaseCannotBeRenewed(t *testing.T) {
	t.Parallel()
	service, db := newTestService(t, 900*time.Millisecond)
	if _, err := service.Start(t.Context()); err != nil {
		t.Fatal(err)
	}
	if err := service.Ready(); err != nil {
		t.Fatalf("not ready after start: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	waitUntilNotReady(t, service)
	if err := service.Ready(); err == nil || !errors.Is(err, service.readyErr) {
		t.Fatalf("readiness = %v, want the renewal failure that cleared it", err)
	}
}

func waitUntilNotReady(t *testing.T, service *Service) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	ticker := time.NewTicker(time.Millisecond)
	defer ticker.Stop()
	for service.Ready() == nil {
		select {
		case <-ctx.Done():
			t.Fatal("readiness was not cleared after the lease could not be renewed")
		case <-ticker.C:
		}
	}
}

func TestAFailedRecoveryLeavesTheServiceNotReady(t *testing.T) {
	t.Parallel()
	service, db := newTestService(t, time.Minute)
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Start(t.Context()); err == nil {
		t.Fatal("recovery on a closed database succeeded")
	}
	if err := service.Ready(); err == nil {
		t.Fatal("a service whose recovery failed became ready")
	}
	if err := service.Close(t.Context()); err == nil {
		t.Fatal("a closed database released ownership")
	}
}
