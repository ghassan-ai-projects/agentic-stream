package actions

import (
	"context"
	"errors"
	"os"
	"testing"
)

type partialSafeStopTransport struct {
	sends  int
	closed bool
}

func (t *partialSafeStopTransport) Send(context.Context, []byte) error {
	t.sends++
	return &possiblySentError{err: errors.New("partial safe-stop write")}
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
	catalogData, err := os.ReadFile("../contractsv1/conformance/v1/thermal-capability-catalog.json")
	if err != nil {
		t.Fatal(err)
	}
	catalog, err := LoadCapabilityCatalog(catalogData)
	if err != nil {
		t.Fatal(err)
	}
	transport := &partialSafeStopTransport{}
	session := &DeviceSession{
		transport: transport, catalog: catalog, deviceID: "thermal-01", bootID: "boot-A",
		authorityEpoch: "epoch-1", ownerInstance: "instance-1", opened: true,
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
