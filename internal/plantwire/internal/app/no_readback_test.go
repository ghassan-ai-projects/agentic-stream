package app_test

import (
	"crypto/sha256"
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/actionport"
	"github.com/ghassan-ai-projects/agentic-stream/internal/plantwire/internal/app"
	"github.com/ghassan-ai-projects/agentic-stream/internal/testsupport/plantwirefake"
)

func (d deviceAuthority) ledger(t *testing.T, commandID, status string) {
	t.Helper()
	if _, err := d.db.ExecContext(t.Context(), "PRAGMA foreign_keys = OFF"); err != nil {
		t.Fatal(err)
	}
	key := sha256.Sum256([]byte(commandID))
	if _, err := d.db.ExecContext(t.Context(), `INSERT INTO commands (command_id, intent_id, tenant_id, effector_route, normalized_target,
		idempotency_key, command_json, command_sha256, status, created_at, updated_at)
		VALUES (?1, 'intent-' || ?1, 'tenant', 'route', 'station-1', ?2, x'7b7d', ?3, ?4, 'now', 'now')
		ON CONFLICT(command_id) DO UPDATE SET status = ?4`, commandID, key[:], make([]byte, 32), status); err != nil {
		t.Fatalf("record %s as %s in the action ledger: %v", commandID, status, err)
	}
}

func unknownOutcome(t *testing.T, host runningHost, node *plantwirefake.Node, command actionport.Command) {
	t.Helper()
	done := dispatch(host, command)
	node.Expect("DCMD")
	node.Send(receipt(command.CommandID, "accepted", ""))
	node.Send(result(command.CommandID, "unknown"))
	if got := answered(t, done); !actionport.IsUnknownOutcome(got.err) {
		t.Fatalf("%s = %v, want an unknown outcome", command.CommandID, got.err)
	}
}

func TestAnUnknownOutcomeWithNothingToReadBackConcludesAsManualReviewAtOnce(t *testing.T) {
	t.Parallel()
	host, node, devices := commandingHostWith(t)
	command := pumpCommand("c-1")
	unknownOutcome(t, host, node, command)
	status, evidence := verify(t, host, command)
	if status != actionport.CommandManualReview || evidence["evidence_type"] != "device_state_feedback" || evidence["boot_id"] != firstBoot {
		t.Fatalf("VerifyDeviceCommand = %q, %v; want manual review with sealed device-state evidence", status, evidence)
	}
	if bound := devices.count(t, "SELECT COUNT(*) FROM device_command_bindings WHERE command_id = 'c-1' AND no_readback = 1"); bound != 1 {
		t.Fatalf("no-readback bindings of c-1 = %d, want 1", bound)
	}
}

func TestAnUnknownOutcomeWithOutputsToReadWaitsForTheirReadingsAndIsBoundAsReadable(t *testing.T) {
	t.Parallel()
	host, node, devices := commandingHostWith(t)
	command := setpointCommand("c-1")
	unknownOutcome(t, host, node, command)
	host.clock.Advance(app.VerificationWindow)
	if status, evidence := verify(t, host, command); status != "" || evidence != nil {
		t.Fatalf("VerifyDeviceCommand = %q, %v; want no final status", status, evidence)
	}
	if bound := devices.count(t, "SELECT COUNT(*) FROM device_command_bindings WHERE command_id = 'c-1' AND no_readback = 0"); bound != 1 {
		t.Fatalf("readable bindings of c-1 = %d, want 1", bound)
	}
}

func TestADeviceWhoseNoReadbackCommandWaitsInManualReviewClearsByReadingsAndTakesTheNextCommand(t *testing.T) {
	t.Parallel()
	host, node, devices := commandingHostWith(t)
	unknownOutcome(t, host, node, pumpCommand("c-1"))
	devices.ledger(t, "c-1", actionport.CommandManualReview)
	assertRefusedForReconciliation(t, host, node, "c-2")
	host.clock.Advance(time.Millisecond)
	node.Send(setpointData(firstBoot, 60, "good", false))
	settled(t, node)
	assertSentOn(t, host, node, "c-3", firstBoot)
}

func TestADeviceWithAnUnresolvedReadableCommandStaysBlockedUntilItIsReconciled(t *testing.T) {
	t.Parallel()
	host, node, devices := commandingHostWith(t)
	unknownOutcome(t, host, node, setpointCommand("c-1"))
	devices.ledger(t, "c-1", actionport.CommandReconciling)
	host.clock.Advance(time.Millisecond)
	node.Send(setpointData(firstBoot, 60, "good", false))
	settled(t, node)
	assertRefusedForReconciliation(t, host, node, "c-2")
	devices.ledger(t, "c-1", actionport.CommandSucceeded)
	assertSentOn(t, host, node, "c-3", firstBoot)
}
