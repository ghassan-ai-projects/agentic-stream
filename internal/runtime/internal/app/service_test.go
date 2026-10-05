package app

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	runtimecontrol "github.com/ghassan-ai-projects/agentic-stream/internal/control"

	"github.com/ghassan-ai-projects/agentic-stream/internal/evidence"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
)

func TestServiceReadinessFollowsRecoveryAndClose(t *testing.T) {
	db, err := storage.Open(t.Context(), filepath.Join(t.TempDir(), "service.db"))
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	defer func() { _ = db.Close() }()
	now := time.Date(2026, 8, 12, 12, 0, 0, 0, time.UTC)
	epoch := "epoch-service"
	owner := &runtimecontrol.RuntimeOwner{DB: db, InstanceID: "instance-service", Lease: time.Minute, Now: func() time.Time { return now }}
	ledger := &evidence.Ledger{DB: db, LeaseOwner: "instance-service", RuntimeEpoch: epoch, Lease: time.Minute}
	service, err := NewService(owner, ledger, epoch)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	if err := service.Ready(); err == nil {
		t.Fatal("service was ready before start")
	}
	if _, err := service.Start(t.Context()); err != nil {
		t.Fatalf("start service: %v", err)
	}
	if err := service.Ready(); err != nil {
		t.Fatalf("service not ready after start: %v", err)
	}
	if err := service.Close(context.Background()); err != nil {
		t.Fatalf("close service: %v", err)
	}
	if err := service.Ready(); err == nil {
		t.Fatal("service remained ready after close")
	}
}

func TestServiceConfigurationAndAbsentLifecycle(t *testing.T) {
	t.Parallel()
	if _, err := NewService(nil, nil, ""); err == nil {
		t.Fatal("missing service configuration accepted")
	}
	var service *Service
	if _, err := service.Start(t.Context()); err == nil {
		t.Fatal("nil service started")
	}
	if service.Ready() == nil || service.Close(t.Context()) != nil {
		t.Fatal("nil lifecycle behavior changed")
	}
}

func TestServiceLeaseFailureClearsReadiness(t *testing.T) {
	db, err := storage.Open(t.Context(), filepath.Join(t.TempDir(), "lease.db"))
	if err != nil {
		t.Fatal(err)
	}
	owner := &runtimecontrol.RuntimeOwner{DB: db, InstanceID: "instance", Lease: time.Minute}
	ledger := &evidence.Ledger{DB: db, LeaseOwner: "instance", RuntimeEpoch: "epoch", Lease: time.Minute}
	service, err := NewService(owner, ledger, "epoch")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = service.Start(t.Context()); err != nil {
		t.Fatal(err)
	}
	if err = service.maintainRuntimeLease(t.Context()); err != nil {
		t.Fatalf("healthy renewal: %v", err)
	}
	if err = db.Close(); err != nil {
		t.Fatal(err)
	}
	// Readiness must expose the actual renewal failure, not retain the earlier ready state.
	err = service.maintainRuntimeLease(t.Context())
	if err == nil {
		t.Fatal("closed database renewed ownership")
	}
	service.setNotReady(err)
	if got := service.Ready(); !errors.Is(got, err) {
		t.Fatalf("readiness=%v want lease error %v", got, err)
	}
	if err = service.Close(t.Context()); err == nil {
		t.Fatal("closed database released ownership")
	}
	if _, err = service.Start(t.Context()); err == nil || service.Ready() == nil {
		t.Fatal("failed recovery became ready")
	}
}
