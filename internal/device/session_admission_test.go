package device

import (
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/actionport"
	"github.com/ghassan-ai-projects/agentic-stream/internal/authority"
)

func TestOrdinaryCommandAdmissionKeepsSafetyPrecedence(t *testing.T) {
	t.Parallel()
	catalog := admissionCatalog(t)
	command, err := catalog.Materialize(actionport.Command{CommandID: "cmd-1", IdempotencyKey: "sha256:" + strings.Repeat("a", 64), PolicyDigest: "sha256:" + strings.Repeat("b", 64), EffectorRoute: "set_indicator", NormalizedTarget: "led-01", Payload: map[string]any{"state": "alert"}}, "boot-A")
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name       string
		open, stop bool
		document   map[string]any
		want       string
	}{
		{name: "closed before priority", stop: true, want: "device session is not open"},
		{name: "priority before decoding", open: true, stop: true, want: "safe stop has priority over ordinary device commands"},
		{name: "boot before barrier", open: true, document: command, want: "device boot changed"},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			transport := &partialSafeStopTransport{}
			session := &DeviceSession{opened: test.open, safeStopRequested: test.stop, bootID: "boot-B", reconciliationRequired: true, transport: transport}
			_, sent, err := session.ExchangeWithResult(t.Context(), test.document)
			if sent || err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("sent=%v, error=%v, want %q", sent, err, test.want)
			}
			if transport.sends != 0 {
				t.Fatalf("rejected admission sent %d commands", transport.sends)
			}
		})
	}
	session := &DeviceSession{opened: true, bootID: "boot-A", reconciliationRequired: true, transport: &partialSafeStopTransport{}}
	_, sent, err := session.ExchangeWithResult(t.Context(), command)
	if sent || !errors.Is(err, authority.ErrReconciliationRequired) {
		t.Fatalf("barrier admission sent=%v, error=%v", sent, err)
	}
}

func TestDeviceDecodeRejectsTrailingDataBeforeMessageSchema(t *testing.T) {
	t.Parallel()
	_, err := DecodeDeviceRecord([]byte("{\"message_type\":\"unsupported\"}\n{}\n"))
	if err == nil || err.Error() != "device frame contains trailing JSON" {
		t.Fatalf("decode error=%v", err)
	}
}

func TestWriteCleanupPreservesOriginalFailureWithoutResetError(t *testing.T) {
	t.Parallel()
	cause := errors.New("original write failure")
	if got := resetWriteDeadline(cause, func() error { return nil }, 1); got != cause { //nolint:errorlint // Exact identity proves cleanup does not introduce a wrapper when no reset fails.
		t.Fatalf("cleanup replaced original failure: %v", got)
	}
	reset := errors.New("deadline reset failure")
	got := resetWriteDeadline(cause, func() error { return reset }, 1)
	if !errors.Is(got, cause) || !errors.Is(got, reset) || !transportMayHaveSent(got) {
		t.Fatalf("cleanup lost cause or sent classification: %v", got)
	}
}

func admissionCatalog(t *testing.T) *CapabilityCatalog {
	t.Helper()
	data, err := os.ReadFile("../contractsv1/conformance/v1/thermal-capability-catalog.json")
	if err != nil {
		t.Fatal(err)
	}
	catalog, err := LoadCapabilityCatalog(data)
	if err != nil {
		t.Fatal(err)
	}
	return catalog
}
