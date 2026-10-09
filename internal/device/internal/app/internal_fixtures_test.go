package app

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/actions"
	deviceauthority "github.com/ghassan-ai-projects/agentic-stream/internal/authority"
	runtimecontrol "github.com/ghassan-ai-projects/agentic-stream/internal/control"
	"github.com/ghassan-ai-projects/agentic-stream/internal/device/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/device/internal/transport"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage/storagetest"
)

type partialSafeStopTransport struct {
	sends  int
	closed bool
}

func (t *partialSafeStopTransport) Send(context.Context, []byte) error {
	t.sends++
	return &transport.PartialSendError{Err: errors.New("partial safe-stop write")}
}

func (t *partialSafeStopTransport) Receive(context.Context) ([]byte, error) {
	return nil, errors.New("partial safe-stop transport has no response")
}

func (t *partialSafeStopTransport) QueryState(context.Context) ([]byte, error) {
	return nil, errors.New("partial safe-stop transport has no state")
}

func (t *partialSafeStopTransport) Close() error {
	t.closed = true
	return nil
}

func thermalCatalog(t *testing.T) *domain.CapabilityCatalog {
	t.Helper()
	data, err := os.ReadFile("../../../contractsv1/internal/domain/conformance/v1/thermal-capability-catalog.json")
	if err != nil {
		t.Fatalf("read catalog: %v", err)
	}
	catalog, err := domain.LoadCapabilityCatalog(data)
	if err != nil {
		t.Fatalf("load catalog: %v", err)
	}
	return catalog
}

func admittedAuthority(t *testing.T) *deviceauthority.Service {
	t.Helper()
	db := storagetest.OpenTemp(t)
	owner := &runtimecontrol.RuntimeOwner{DB: db, InstanceID: "instance-1", Lease: time.Minute}
	if err := owner.Claim(t.Context(), "epoch-1"); err != nil {
		t.Fatalf("claim runtime owner: %v", err)
	}
	authority, err := deviceauthority.New(deviceauthority.Config{DB: db, Owner: owner, Epochs: &runtimecontrol.EpochControl{DB: db}, Outcomes: actions.CountUnresolvedOutcomes})
	if err != nil {
		t.Fatalf("new device authority: %v", err)
	}
	return authority
}
