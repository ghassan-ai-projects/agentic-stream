package runtime

import (
	"context"
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

func TestReadinessFacadeRejectsMissingConfiguration(t *testing.T) {
	t.Parallel()
	if _, err := NewService(nil, nil, ""); err == nil {
		t.Fatal("missing service dependencies accepted")
	}
	var service *Service
	if _, err := service.Start(t.Context()); err == nil {
		t.Fatal("nil service started")
	}
	if service.Ready() == nil {
		t.Fatal("nil service ready")
	}
	if err := service.Close(t.Context()); err != nil {
		t.Fatal(err)
	}
}
