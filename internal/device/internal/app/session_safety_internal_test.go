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
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"

	"github.com/ghassan-ai-projects/agentic-stream/internal/device/internal/domain"

	"github.com/ghassan-ai-projects/agentic-stream/internal/device/internal/transport"
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

func TestSafeStopPossiblySentFailureInvalidatesTransport(t *testing.T) {
	catalogData, err := os.ReadFile("../../../contractsv1/conformance/v1/thermal-capability-catalog.json")
	if err != nil {
		t.Fatal(err)
	}
	catalog, err := domain.LoadCapabilityCatalog(catalogData)
	if err != nil {
		t.Fatal(err)
	}
	transport := &partialSafeStopTransport{}
	session := &Session{
		transport: transport, catalog: catalog, deviceID: "thermal-01", bootID: "boot-A",
		ownerEpoch: "epoch-1", ownerInstance: "instance-1", opened: true, authority: testAuthority(t),
	}
	defer func() { _ = session.Close() }()

	_, sent, err := session.SafeStop(context.Background(), "fan-01")
	if err == nil || !sent {
		t.Fatalf("partial safe-stop send sent=%v err=%v", sent, err)
	}
	if !transport.closed {
		t.Fatal("partial safe-stop send left transport open")
	}
	if _, sent, nextErr := session.SafeStop(context.Background(), "fan-01"); nextErr == nil || sent {
		t.Fatalf("safe-stop retried on invalidated transport sent=%v err=%v", sent, nextErr)
	}
	if transport.sends != 1 {
		t.Fatalf("safe-stop sends=%d, want 1", transport.sends)
	}
}

// testAuthority is a device authority for an admitted epoch-1 of instance-1.
func testAuthority(t *testing.T) *deviceauthority.Service {
	t.Helper()
	db, err := storage.Open(context.Background(), t.TempDir()+"/device.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	owner := &runtimecontrol.RuntimeOwner{DB: db, InstanceID: "instance-1", Lease: time.Minute}
	if err := owner.Claim(context.Background(), "epoch-1"); err != nil {
		t.Fatal(err)
	}
	authority, err := deviceauthority.New(deviceauthority.Config{DB: db, Owner: owner, Epochs: &runtimecontrol.EpochControl{DB: db}, Outcomes: actions.CountUnresolvedOutcomes})
	if err != nil {
		t.Fatal(err)
	}
	return authority
}
