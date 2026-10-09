package app_test

import (
	"context"
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/actionport"
	"github.com/ghassan-ai-projects/agentic-stream/internal/device/internal/app"
)

func TestAMalformedReceiptAfterSendingIsAnUnknownOutcomeThatSurvivesRestart(t *testing.T) {
	t.Parallel()
	session, transport, catalog, control := openThermalSessionWithControl(t)
	transport.queue([]byte(`{"message_type":"receipt"}`))
	_, err := app.NewGatewayEffector(session, catalog).Dispatch(t.Context(), indicatorCommand("cmd-1"))
	if !actionport.IsUnknownOutcome(err) {
		t.Fatalf("malformed post-send receipt err=%v, want an unknown outcome", err)
	}
	if transport.sendCount() != 1 || !reconciliationRequired(t, control) {
		t.Fatalf("sends=%d barrier=%v; want 1 send and an open barrier", transport.sendCount(), reconciliationRequired(t, control))
	}
	if cleared, err := session.ResolveReconciliation(t.Context(), "succeeded", map[string]any{"state_digest": "stale"}); cleared || err == nil {
		t.Fatalf("reconciliation resolved without a fresh state query: cleared=%v err=%v", cleared, err)
	}
	assertRestartedSessionRefusesOrdinaryCommands(t, control, catalog, stateFor(t, catalog), "reconciliation")
}

func TestAResultThatCannotBeTrustedKeepsTheReceiptAndRequiresReconciliation(t *testing.T) {
	t.Parallel()
	session, transport, catalog, control := openThermalSessionWithControl(t)
	transport.queue(mustEncode(t, acceptedReceipt("cmd-result-bad")), []byte("{"))
	effect, err := app.NewGatewayEffector(session, catalog).Dispatch(t.Context(), indicatorCommand("cmd-result-bad"))
	if !actionport.IsUnknownOutcome(err) {
		t.Fatalf("untrustworthy result err=%v, want an unknown outcome", err)
	}
	receipt, _ := effect.ProviderResult["receipt"].(map[string]any)
	if receipt["command_id"] != "cmd-result-bad" || len(providerMap(effect, "result")) != 0 {
		t.Fatalf("partial provider evidence=%#v", effect.ProviderResult)
	}
	if !transport.isClosed() || !reconciliationRequired(t, control) {
		t.Fatalf("link closed=%v, barrier=%v; want both", transport.isClosed(), reconciliationRequired(t, control))
	}
	_, sent, nextErr := session.Exchange(t.Context(), materializedCommand(t, catalog, "cmd-after-bad-result", idemKey()))
	assertRefusedBeforeSend(t, sent, nextErr, "device session is not open")
}

func TestCancellationAfterSendingStillPersistsTheReconciliationBarrier(t *testing.T) {
	t.Parallel()
	session, transport, catalog, control := openThermalSessionWithControl(t)
	transport.receiveErr = context.Canceled
	ctx, cancel := context.WithCancel(t.Context())
	transport.sendHook = cancel
	_, err := app.NewGatewayEffector(session, catalog).Dispatch(ctx, indicatorCommand("cmd-canceled"))
	if !actionport.IsUnknownOutcome(err) || !reconciliationRequired(t, control) {
		t.Fatalf("canceled receipt err=%v barrier=%v; want an unknown outcome and an open barrier", err, reconciliationRequired(t, control))
	}
}
