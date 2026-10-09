package store

import (
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/actions/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/kernel"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
)

func cloneCommand(t *testing.T, db *storage.DB, commandID string, availableAt time.Time) {
	t.Helper()
	execute(t, db, "PRAGMA foreign_keys = OFF")
	defer execute(t, db, "PRAGMA foreign_keys = ON")
	execute(t, db, `
		INSERT INTO intents (intent_id, decision_id, tenant_id, situation_id, situation_version, intent_type, risk_class,
			intent_json, intent_sha256, expires_at, policy_status, created_at, updated_at)
		SELECT 'int-' || ?, decision_id, tenant_id, situation_id, situation_version, intent_type, risk_class,
			intent_json, randomblob(32), expires_at, policy_status, created_at, updated_at FROM intents WHERE intent_id = 'int-action'`, commandID)
	execute(t, db, `
		INSERT INTO commands (command_id, intent_id, tenant_id, effector_route, normalized_target, idempotency_key,
			command_json, command_sha256, status, created_at, updated_at)
		SELECT ?, 'int-' || ?, tenant_id, effector_route, normalized_target, randomblob(32),
			command_json, command_sha256, 'pending', created_at, updated_at FROM commands WHERE command_id = 'cmd-action'`, commandID, commandID)
	execute(t, db, `
		INSERT INTO outbox (kind, aggregate_id, aggregate_version, payload_json, status, available_at, created_at)
		SELECT kind, ?, aggregate_version, payload_json, 'pending', ?, created_at FROM outbox WHERE aggregate_id = 'cmd-action'`,
		commandID, kernel.FormatTime(availableAt))
}

func nextCommandID(t *testing.T, db *storage.DB, now time.Time) string {
	t.Helper()
	var commandID string
	inTx(t, db, func(tx *Tx) error {
		candidate, found, err := tx.NextCandidate(t.Context(), now)
		if err != nil {
			t.Fatal(err)
		}
		if found {
			commandID = candidate.Command.ID
		}
		return nil
	})
	return commandID
}

func TestTheNextCandidateIsTheOldestOutboxRowAvailableAtThatTime(t *testing.T) {
	t.Parallel()
	db, commandID := openActionFixture(t)
	cloneCommand(t, db, "cmd-later", testNow.Add(-30*time.Second))
	cloneCommand(t, db, "cmd-future", testNow.Add(time.Hour))

	if got := nextCommandID(t, db, testNow.Add(-time.Hour)); got != "" {
		t.Fatalf("candidate before any row is available = %q, want none", got)
	}
	if got := nextCommandID(t, db, testNow); got != commandID {
		t.Fatalf("first candidate = %q, want the oldest row %q", got, commandID)
	}
	execute(t, db, "UPDATE outbox SET status = 'delivered' WHERE aggregate_id = ?", commandID)
	if got := nextCommandID(t, db, testNow); got != "cmd-later" {
		t.Fatalf("next candidate = %q, want cmd-later: a delivered row is never offered again", got)
	}
	execute(t, db, "UPDATE outbox SET status = 'delivered' WHERE aggregate_id = 'cmd-later'")
	if got := nextCommandID(t, db, testNow); got != "" {
		t.Fatalf("candidate = %q, want none before cmd-future is available", got)
	}
	if got := nextCommandID(t, db, testNow.Add(2*time.Hour)); got != "cmd-future" {
		t.Fatalf("candidate = %q, want cmd-future once available", got)
	}
}

func TestACandidateCarriesTheLedgerColumnsItsCommandDocumentMustMatch(t *testing.T) {
	t.Parallel()
	db, commandID := openActionFixture(t)
	inTx(t, db, func(tx *Tx) error {
		candidate, found, err := tx.NextCandidate(t.Context(), testNow)
		if err != nil || !found {
			t.Fatalf("candidate found=%v err=%v", found, err)
		}
		if candidate.Command.ID != commandID || candidate.OutboxStatus != "pending" || candidate.CommandStatus != "pending" ||
			candidate.Command.TenantID != "tenant" || candidate.Command.IntentID != "int-action" || candidate.Command.Route != "maintenance.ticket" ||
			candidate.Command.Target != "motor/1" || len(candidate.Command.Idempotency) != 32 || candidate.Lease.HasOwner || candidate.Lease.HasUntil {
			t.Fatalf("candidate = %+v", candidate)
		}
		return nil
	})
}

func TestInvalidAndTerminalCommandsCloseTheirOutbox(t *testing.T) {
	t.Parallel()
	db, commandID := openActionFixture(t)
	var outboxID int64
	inTx(t, db, func(tx *Tx) error {
		candidate, _, err := tx.NextCandidate(t.Context(), testNow)
		outboxID = candidate.OutboxID
		if err != nil {
			return err
		}
		return tx.FailInvalidCommand(t.Context(), outboxID, commandID, domain.FailureCommandDigestMismatch, testNow)
	})
	read := func() (command, outbox, code string) {
		row := db.QueryRowContext(t.Context(), `SELECT c.status, o.status, COALESCE(o.last_error_code, '') FROM commands c JOIN outbox o ON o.aggregate_id = c.command_id WHERE c.command_id = ?`, commandID)
		if err := row.Scan(&command, &outbox, &code); err != nil {
			t.Fatal(err)
		}
		return command, outbox, code
	}
	if command, outbox, code := read(); command != "failed" || outbox != "failed" || code != domain.FailureCommandDigestMismatch {
		t.Fatalf("invalid command ledger = %q %q %q", command, outbox, code)
	}
	inTx(t, db, func(tx *Tx) error { return tx.CloseOutboxOnly(t.Context(), outboxID, domain.OutboxDelivered, testNow) })
	if _, outbox, _ := read(); outbox != "delivered" {
		t.Fatalf("outbox status = %q, want delivered", outbox)
	}
}

func TestOnlyACommandNotYetSettledIsMarkedDispatching(t *testing.T) {
	t.Parallel()
	for status, want := range map[string]string{
		"pending": "dispatching", "succeeded": "succeeded", "reconciling": "reconciling",
		"manual_review": "manual_review", "outcome_unknown": "outcome_unknown",
	} {
		t.Run(status, func(t *testing.T) {
			t.Parallel()
			db, commandID := openActionFixture(t)
			execute(t, db, "UPDATE commands SET status = ?", status)
			inTx(t, db, func(tx *Tx) error { return tx.MarkCommandDispatching(t.Context(), commandID, testNow) })
			if got := queryString(t, db, "SELECT status FROM commands"); got != want {
				t.Fatalf("status after MarkCommandDispatching from %q = %q, want %q", status, got, want)
			}
		})
	}
}
