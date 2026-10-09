package app_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/actionport"
	"github.com/ghassan-ai-projects/agentic-stream/internal/device/internal/app"
)

func providerMap(effect actionport.Effect, key string) map[string]any {
	document, _ := effect.ProviderResult[key].(map[string]any)
	return document
}

func resultStatus(effect actionport.Effect) (status, errorCode any) {
	result, _ := effect.ProviderResult["result"].(map[string]any)
	return result["status"], result["error_code"]
}

func TestSafeStopRejectedByTheDeviceKeepsTheKnownEvidenceAndLatchesTheStop(t *testing.T) {
	t.Parallel()
	session, _, catalog := openThermalSession(t, rejectedReceipt("safe-stop/fan-01", "not_ready"))
	effect, err := app.NewGatewayEffector(session, catalog).SafeStop(t.Context(), "fan-01")
	if err == nil || actionport.IsUnknownOutcome(err) {
		t.Fatalf("known safe-stop rejection err=%v", err)
	}
	if status, code := resultStatus(effect); status != "rejected" || code != "not_ready" || effect.VerificationPending {
		t.Fatalf("safe-stop rejection evidence=%#v pending=%v", effect.ProviderResult, effect.VerificationPending)
	}
	_, sent, ordinaryErr := session.Exchange(t.Context(), materializedCommand(t, catalog, "cmd-after-safe-stop-rejection", idemKey()))
	assertRefusedBeforeSend(t, sent, ordinaryErr, "safe stop has priority")
}

func TestSafeStopReceiveFailureIsUnknownAndInvalidatesTheLink(t *testing.T) {
	t.Parallel()
	session, transport, catalog, control := openThermalSessionWithControl(t)
	transport.receiveErr = errors.New("safe-stop receipt timeout")
	_, err := app.NewGatewayEffector(session, catalog).SafeStop(t.Context(), "fan-01")
	if !actionport.IsUnknownOutcome(err) {
		t.Fatalf("safe-stop receive failure err=%v, want an unknown outcome", err)
	}
	if !transport.isClosed() || !reconciliationRequired(t, control) {
		t.Fatalf("link closed=%v, barrier=%v; want both", transport.isClosed(), reconciliationRequired(t, control))
	}
	if _, sent, nextErr := session.SafeStop(t.Context(), "fan-01"); nextErr == nil || sent {
		t.Fatalf("safe-stop retried on an invalidated link: sent=%v err=%v", sent, nextErr)
	}
	if transport.sendCount() != 1 {
		t.Fatalf("safe-stop sends=%d, want 1", transport.sendCount())
	}
}

func TestSafeStopWithAnUntrustworthyResultKeepsTheReceiptAndRequiresReconciliation(t *testing.T) {
	t.Parallel()
	session, transport, catalog, control := openThermalSessionWithControl(t)
	transport.queue(mustEncode(t, acceptedReceipt("safe-stop/fan-01")), []byte("{"))
	effect, err := app.NewGatewayEffector(session, catalog).SafeStop(t.Context(), "fan-01")
	if !actionport.IsUnknownOutcome(err) {
		t.Fatalf("untrustworthy safe-stop result err=%v, want an unknown outcome", err)
	}
	receipt, _ := effect.ProviderResult["receipt"].(map[string]any)
	if receipt["command_id"] != "safe-stop/fan-01" || len(providerMap(effect, "result")) != 0 {
		t.Fatalf("partial safe-stop evidence=%#v", effect.ProviderResult)
	}
	if !transport.isClosed() || !reconciliationRequired(t, control) {
		t.Fatalf("link closed=%v, barrier=%v; want both", transport.isClosed(), reconciliationRequired(t, control))
	}
}

func TestSafeStopWithAMalformedReceiptOpensTheReconciliationBarrier(t *testing.T) {
	t.Parallel()
	session, transport, catalog, control := openThermalSessionWithControl(t)
	transport.queue([]byte("{"))
	_, err := app.NewGatewayEffector(session, catalog).SafeStop(t.Context(), "fan-01")
	if !actionport.IsUnknownOutcome(err) || !reconciliationRequired(t, control) {
		t.Fatalf("malformed safe-stop receipt err=%v, barrier=%v", err, reconciliationRequired(t, control))
	}
}

func TestSafeStopRejectionThatCannotBeRecordedIsUnknownAndSurvivesRestart(t *testing.T) {
	t.Parallel()
	session, transport, catalog, control := openThermalSessionWithControl(t, rejectedReceipt("safe-stop/fan-01", "not_ready"))
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	transport.sendHook = cancel
	effect, err := app.NewGatewayEffector(session, catalog).SafeStop(ctx, "fan-01")
	if !actionport.IsUnknownOutcome(err) {
		t.Fatalf("undurable safe-stop rejection err=%v, want an unknown outcome", err)
	}
	if status, code := resultStatus(effect); status != "rejected" || code != "not_ready" {
		t.Fatalf("undurable rejection evidence=%#v", effect.ProviderResult)
	}
	if !transport.isClosed() || !reconciliationRequired(t, control) {
		t.Fatalf("link closed=%v, barrier=%v; want both", transport.isClosed(), reconciliationRequired(t, control))
	}
	if failed := countAuthorityEvents(t, control, "safe_stop_failed"); failed != 0 {
		t.Fatalf("undurable rejection recorded %d safe-stop failure events", failed)
	}
	if err := session.Close(); err != nil {
		t.Fatalf("close session: %v", err)
	}
	assertRestartedSessionRefusesOrdinaryCommands(t, control, catalog, stateFor(t, catalog), "safe stop has priority")
}

func TestASafeStopAcceptedWithoutDurableEvidenceRequiresReconciliation(t *testing.T) {
	t.Parallel()
	session, transport, catalog, control := openThermalSessionWithControl(t, acceptedReceipt("safe-stop/fan-01"))
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	transport.sendHook = cancel
	_, err := app.NewGatewayEffector(session, catalog).SafeStop(ctx, "fan-01")
	if err == nil || !strings.Contains(err.Error(), "safe-stop accepted but lifecycle evidence was not durable") {
		t.Fatalf("accepted safe stop without durable evidence err=%v", err)
	}
	if !reconciliationRequired(t, control) {
		t.Fatal("an accepted safe stop with lost evidence left no reconciliation barrier")
	}
	if completed := countAuthorityEvents(t, control, "safe_stop_completed"); completed != 0 {
		t.Fatalf("recorded %d completions for a stop whose evidence was lost", completed)
	}
}
