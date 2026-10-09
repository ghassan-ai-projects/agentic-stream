package runtime_test

import (
	"testing"
	"time"

	runtimecontrol "github.com/ghassan-ai-projects/agentic-stream/internal/control"
	"github.com/ghassan-ai-projects/agentic-stream/internal/evidence"
	"github.com/ghassan-ai-projects/agentic-stream/internal/runtime"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage/storagetest"
)

func TestServiceFacadeIsReadyOnlyBetweenStartAndClose(t *testing.T) {
	t.Parallel()
	db := storagetest.OpenTemp(t)
	owner := &runtimecontrol.RuntimeOwner{DB: db, InstanceID: "instance-service", Lease: time.Minute}
	ledger, err := evidence.New(evidence.Config{Ledger: &evidence.LedgerConfig{OwnerCheck: owner.Assert, DB: db, LeaseOwner: "instance-service", RuntimeEpoch: "epoch-service", Lease: time.Minute}})
	if err != nil {
		t.Fatal(err)
	}
	service, err := runtime.NewService(owner, ledger, "epoch-service")
	if err != nil {
		t.Fatal(err)
	}
	if service.Ready() == nil {
		t.Fatal("service was ready before start")
	}
	if _, err := service.Start(t.Context()); err != nil {
		t.Fatalf("start: %v", err)
	}
	if err := service.Ready(); err != nil {
		t.Fatalf("not ready after start: %v", err)
	}
	if err := service.Close(t.Context()); err != nil {
		t.Fatalf("close: %v", err)
	}
	if service.Ready() == nil {
		t.Fatal("service remained ready after close")
	}
}

func TestServiceFacadeRequiresItsConfigurationAndToleratesAnAbsentInstance(t *testing.T) {
	t.Parallel()
	if _, err := runtime.NewService(nil, nil, ""); err == nil {
		t.Fatal("missing service dependencies accepted")
	}
	var service *runtime.Service
	if _, err := service.Start(t.Context()); err == nil {
		t.Fatal("nil service started")
	}
	if service.Ready() == nil || service.Close(t.Context()) != nil {
		t.Fatal("a nil service must report not ready and close without error")
	}
}
