package app

import (
	"errors"
	"strings"
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/actionport"
	"github.com/ghassan-ai-projects/agentic-stream/internal/authority"
	"github.com/ghassan-ai-projects/agentic-stream/internal/device/internal/domain"
)

func TestOrdinaryCommandAdmissionKeepsSafetyPrecedence(t *testing.T) {
	t.Parallel()
	command := admissionCommand(t)
	cases := []struct {
		name     string
		session  *Session
		document domain.Command
		want     string
	}{
		{"closed before priority", &Session{safeStopLatched: true}, domain.Command{}, "device session is not open"},
		{"priority before decoding", &Session{opened: true, safeStopLatched: true}, domain.Command{}, "safe stop has priority over ordinary device commands"},
		{"boot before barrier", &Session{opened: true, bootID: "boot-B", reconciliationRequired: true}, command, "device boot changed"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			transport := &partialSafeStopTransport{}
			tc.session.transport = transport
			_, sent, err := tc.session.Exchange(t.Context(), tc.document)
			if sent || err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("sent=%v, error=%v, want an unsent refusal containing %q", sent, err, tc.want)
			}
			if transport.sends != 0 {
				t.Fatalf("rejected admission sent %d commands", transport.sends)
			}
		})
	}
}

func TestAnOpenReconciliationBarrierRefusesAnOrdinaryCommandOfTheCurrentBoot(t *testing.T) {
	t.Parallel()
	transport := &partialSafeStopTransport{}
	session := &Session{opened: true, bootID: "boot-A", reconciliationRequired: true, transport: transport}
	_, sent, err := session.Exchange(t.Context(), admissionCommand(t))
	if sent || !errors.Is(err, authority.ErrReconciliationRequired) || transport.sends != 0 {
		t.Fatalf("barrier admission sent=%v, error=%v, sends=%d", sent, err, transport.sends)
	}
}

func admissionCommand(t *testing.T) domain.Command {
	t.Helper()
	command, err := thermalCatalog(t).Materialize(actionport.Command{
		CommandID: "cmd-1", IdempotencyKey: "sha256:" + strings.Repeat("a", 64), PolicyDigest: "sha256:" + strings.Repeat("b", 64),
		EffectorRoute: "set_indicator", NormalizedTarget: "led-01", Payload: map[string]any{"state": "alert"},
	}, "boot-A")
	if err != nil {
		t.Fatalf("materialize admission command: %v", err)
	}
	return command
}
