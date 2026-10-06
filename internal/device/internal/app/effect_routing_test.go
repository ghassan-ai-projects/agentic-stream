package app_test

import (
	"context"
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/actionport"
	"github.com/ghassan-ai-projects/agentic-stream/internal/device/internal/app"
)

func TestGatewayEffectorDispatchesThermalRoute(t *testing.T) {
	session, transport, catalog := openThermalSession(t, acceptedReceipt("cmd-thermal"))
	defer func() { _ = session.Close() }()
	effector := app.NewGatewayEffector(session, catalog)
	effect, err := effector.Dispatch(context.Background(), actionport.Command{
		CommandID: "cmd-thermal", EffectorRoute: "set_indicator", NormalizedTarget: "led-01",
		IdempotencyKey: idemKey(), PolicyDigest: policyKey(), Payload: map[string]any{"state": "watch"},
	})
	if err != nil || !effect.VerificationPending || transport.sendCount() != 1 {
		t.Fatalf("thermal dispatch effect=%v err=%v sends=%d", effect, err, transport.sendCount())
	}
}
