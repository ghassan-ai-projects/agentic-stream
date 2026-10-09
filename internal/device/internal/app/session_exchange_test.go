package app_test

import (
	"errors"
	"strings"
	"testing"
)

func TestSessionAnswersADuplicateIdempotencyKeyFromItsReceiptCache(t *testing.T) {
	t.Parallel()
	session, transport, catalog := openThermalSession(t, acceptedReceipt("cmd-1"))
	idempotency := idemKey()
	first, sent, err := session.Exchange(t.Context(), materializedCommand(t, catalog, "cmd-1", idempotency))
	if err != nil || !sent || first.Receipt.CommandID != "cmd-1" {
		t.Fatalf("first exchange receipt=%v sent=%v err=%v", first, sent, err)
	}
	second, sent, err := session.Exchange(t.Context(), materializedCommand(t, catalog, "cmd-2", idempotency))
	if err != nil || !sent || second.Receipt.CommandID != "cmd-1" {
		t.Fatalf("duplicate exchange receipt=%v sent=%v err=%v", second, sent, err)
	}
	if got := transport.sendCount(); got != 1 {
		t.Fatalf("transport sends=%d, want 1", got)
	}
}

func TestSessionRefusesAnIdempotencyKeyReusedForADifferentCommand(t *testing.T) {
	t.Parallel()
	session, transport, catalog := openThermalSession(t, acceptedReceipt("cmd-1"))
	idempotency := idemKey()
	if _, _, err := session.Exchange(t.Context(), materializedCommand(t, catalog, "cmd-1", idempotency)); err != nil {
		t.Fatalf("first exchange: %v", err)
	}
	conflicting := materializedCommand(t, catalog, "cmd-3", idempotency)
	conflicting.Parameters = map[string]any{"brightness_permille": 1, "pattern": "off"}
	_, sent, err := session.Exchange(t.Context(), conflicting)
	assertRefusedBeforeSend(t, sent, err, "conflicts with the prior device command")
	if got := transport.sendCount(); got != 1 {
		t.Fatalf("transport sends=%d, want 1", got)
	}
}

func TestSessionTreatsAFailureAfterSendingAsAnAmbiguousOutcome(t *testing.T) {
	t.Parallel()
	session, transport, catalog := openThermalSession(t)
	transport.receiveErr = errors.New("gateway receive timeout")
	_, sent, err := session.Exchange(t.Context(), materializedCommand(t, catalog, "cmd-1", idemKey()))
	if err == nil || !sent || !strings.Contains(err.Error(), "gateway receive timeout") {
		t.Fatalf("post-send failure sent=%v err=%v", sent, err)
	}
}

func TestSessionTreatsAFailureBeforeSendingAsUnsent(t *testing.T) {
	t.Parallel()
	session, transport, catalog := openThermalSession(t)
	transport.sendErr = errors.New("gateway refused write")
	_, sent, err := session.Exchange(t.Context(), materializedCommand(t, catalog, "cmd-1", idemKey()))
	assertRefusedBeforeSend(t, sent, err, "gateway refused write")
	if transport.sendCount() != 0 {
		t.Fatalf("transport sends=%d, want 0", transport.sendCount())
	}
}

func TestSessionStopsExchangingAfterAFailedStateRefresh(t *testing.T) {
	t.Parallel()
	session, transport, catalog := openThermalSession(t)
	queueState(t, transport, catalog, map[string]any{"capability_digest": "sha256:" + strings.Repeat("e", 64)})
	if _, err := session.QueryState(t.Context()); err == nil || !strings.Contains(err.Error(), "capability digest") {
		t.Fatalf("refresh with a foreign capability digest = %v", err)
	}
	_, sent, err := session.Exchange(t.Context(), materializedCommand(t, catalog, "cmd-1", idemKey()))
	assertRefusedBeforeSend(t, sent, err, "device session is not open")
}

func TestSessionRefreshToANewBootFencesCachedReceiptsAndOldBootCommands(t *testing.T) {
	t.Parallel()
	session, transport, catalog := openThermalSession(t, acceptedReceipt("cmd-1"))
	command := materializedCommand(t, catalog, "cmd-1", idemKey())
	if _, _, err := session.Exchange(t.Context(), command); err != nil {
		t.Fatalf("exchange on boot-A: %v", err)
	}
	queueState(t, transport, catalog, map[string]any{"boot_id": "boot-B"})
	if _, err := session.QueryState(t.Context()); err != nil {
		t.Fatalf("refresh to boot-B: %v", err)
	}
	if session.BootID() != "boot-B" {
		t.Fatalf("boot id=%q, want boot-B", session.BootID())
	}
	_, sent, err := session.Exchange(t.Context(), command)
	assertRefusedBeforeSend(t, sent, err, "device boot changed")
	if transport.sendCount() != 1 {
		t.Fatalf("transport sends=%d, want only the boot-A command", transport.sendCount())
	}
}

func TestSessionRefusesAMaterializedCommandForAnotherBootBeforeSending(t *testing.T) {
	t.Parallel()
	session, transport, catalog := openThermalSession(t)
	command := materializedCommandWithBoot(t, catalog, "cmd-other-boot", idemKey(), "boot-Z")
	_, sent, err := session.Exchange(t.Context(), command)
	assertRefusedBeforeSend(t, sent, err, "device boot changed")
	if transport.sendCount() != 0 {
		t.Fatalf("transport sends=%d, want 0", transport.sendCount())
	}
}
