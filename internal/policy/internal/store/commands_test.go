package store

import (
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/policy/internal/domain"
)

func prepareCommand(t *testing.T, row domain.IntentRecord, id string) domain.CommandRecord {
	t.Helper()
	command, err := domain.NewCommand(domain.CommandPreparation{ID: id, PolicyDigest: "sha256:policy", Row: row, Intent: domain.ProjectIntent(map[string]any{"parameters": map[string]any{"target": "motor"}}), Now: fixtureNow})
	if err != nil {
		t.Fatal(err)
	}
	return command
}

func TestACommandIsStoredOnceAndAReplayLearnsTheWinner(t *testing.T) {
	t.Parallel()
	db, intentID := openPolicyFixture(t, "R1", 1, 1, fixtureNow.Add(time.Hour))
	inJoined(t, db, func(tx *Tx) error {
		row, err := tx.LoadIntent(t.Context(), intentID)
		if err != nil {
			return err
		}
		command := prepareCommand(t, row, "command-1")

		stored, existing, err := tx.StoreCommandOnce(t.Context(), row, command, fixtureNow)
		if err != nil || stored.ID != command.ID || existing != "" {
			t.Fatalf("first store = %q existing %q, %v; want the new command", stored.ID, existing, err)
		}
		rival := prepareCommand(t, row, "command-2")
		stored, existing, err = tx.StoreCommandOnce(t.Context(), row, rival, fixtureNow)
		if err != nil || stored.ID != "" || existing != command.ID {
			t.Fatalf("second store = %q existing %q, %v; want the winning command %s", stored.ID, existing, err, command.ID)
		}
		if id, err := tx.ExistingCommandID(t.Context(), intentID); err != nil || id != command.ID {
			t.Fatalf("existing command = %q, %v", id, err)
		}
		return nil
	})
	if commands := scalar[int](t, db, "SELECT COUNT(*) FROM commands"); commands != 1 {
		t.Fatalf("commands = %d, want 1", commands)
	}
}

func TestAnIntentWithoutACommandHasNoExistingCommandID(t *testing.T) {
	t.Parallel()
	db, intentID := openPolicyFixture(t, "R1", 1, 1, fixtureNow.Add(time.Hour))
	inJoined(t, db, func(tx *Tx) error {
		if id, err := tx.ExistingCommandID(t.Context(), intentID); err != nil || id != "" {
			t.Fatalf("existing command = %q, %v; want none", id, err)
		}
		return nil
	})
}

func TestACommandIsPublishedToTheOutboxOnce(t *testing.T) {
	t.Parallel()
	db, intentID := openPolicyFixture(t, "R1", 1, 1, fixtureNow.Add(time.Hour))
	inJoined(t, db, func(tx *Tx) error {
		row, err := tx.LoadIntent(t.Context(), intentID)
		if err != nil {
			return err
		}
		command := prepareCommand(t, row, "command-1")
		for range 2 {
			if err := tx.InsertCommandOutbox(t.Context(), command.ID, command.JSON, fixtureNow); err != nil {
				return err
			}
		}
		return nil
	})
	if rows := scalar[int](t, db, "SELECT COUNT(*) FROM outbox WHERE kind = 'command' AND aggregate_id = 'command-1' AND status = 'pending'"); rows != 1 {
		t.Fatalf("pending outbox rows = %d, want 1", rows)
	}
}

func TestTheHourlyDispatchBudgetIsPerTenantTypeAndHour(t *testing.T) {
	t.Parallel()
	db, intentID := openPolicyFixture(t, "R1", 1, 1, fixtureNow.Add(time.Hour))
	inJoined(t, db, func(tx *Tx) error {
		row, err := tx.LoadIntent(t.Context(), intentID)
		if err != nil {
			return err
		}
		row.RateLimitPerHour = 2
		for attempt, wantLimited := range []bool{false, false, true, true} {
			if limited, err := tx.DispatchWithinLimit(t.Context(), row, fixtureNow); err != nil || limited != wantLimited {
				t.Fatalf("dispatch %d at the limit of 2: limited=%t, %v; want limited=%t", attempt+1, limited, err, wantLimited)
			}
		}
		if limited, err := tx.DispatchWithinLimit(t.Context(), row, fixtureNow.Add(time.Hour)); err != nil || limited {
			t.Fatalf("the next hour is limited=%t, %v; want a fresh budget", limited, err)
		}
		row.IntentType = "page"
		if limited, err := tx.DispatchWithinLimit(t.Context(), row, fixtureNow); err != nil || limited {
			t.Fatalf("another intent type is limited=%t, %v; want its own budget", limited, err)
		}
		return nil
	})
	if counted := scalar[int](t, db, "SELECT count FROM intent_dispatch_counts WHERE intent_type = 'create_ticket' AND bucket = ?", fixtureNow.Format("2006-01-02T15:00")); counted != 2 {
		t.Fatalf("recorded dispatches = %d, want the budget of 2 and no more", counted)
	}
}

func TestOnlyTheNamedPendingCommandOfTheIntentIsRemoved(t *testing.T) {
	t.Parallel()
	db, intentID := openPolicyFixture(t, "R1", 1, 1, fixtureNow.Add(time.Hour))
	inJoined(t, db, func(tx *Tx) error {
		row, err := tx.LoadIntent(t.Context(), intentID)
		if err != nil {
			return err
		}
		command := prepareCommand(t, row, "command-1")
		if _, _, err := tx.StoreCommandOnce(t.Context(), row, command, fixtureNow); err != nil {
			return err
		}
		if err := tx.RemovePreparedCommand(t.Context(), intentID, "different"); err == nil {
			t.Error("another command id was removed")
		}
		if err := tx.RemovePreparedCommand(t.Context(), "other-intent", command.ID); err == nil {
			t.Error("a command of another intent was removed")
		}
		if err := tx.RemovePreparedCommand(t.Context(), intentID, command.ID); err != nil {
			t.Errorf("removing the prepared command: %v", err)
		}
		if tenant, found, err := tx.CompensationTenant(t.Context(), command.ID); err != nil || found || tenant != "" {
			t.Errorf("removed command still known: %q %t %v", tenant, found, err)
		}
		return nil
	})
}

func TestACommandThatLeftPendingCannotBeRemoved(t *testing.T) {
	t.Parallel()
	db, intentID := openPolicyFixture(t, "R1", 1, 1, fixtureNow.Add(time.Hour))
	inJoined(t, db, func(tx *Tx) error {
		row, err := tx.LoadIntent(t.Context(), intentID)
		if err != nil {
			return err
		}
		command := prepareCommand(t, row, "command-1")
		if _, _, err := tx.StoreCommandOnce(t.Context(), row, command, fixtureNow); err != nil {
			return err
		}
		if _, err := tx.tx.ExecContext(t.Context(), "UPDATE commands SET status = 'dispatching'"); err != nil {
			return err
		}
		if err := tx.RemovePreparedCommand(t.Context(), intentID, command.ID); err == nil {
			t.Error("a dispatching command was removed")
		}
		return nil
	})
}

func TestACompensationTargetIsKnownByItsTenant(t *testing.T) {
	t.Parallel()
	db, intentID := openPolicyFixture(t, "R1", 1, 1, fixtureNow.Add(time.Hour))
	inJoined(t, db, func(tx *Tx) error {
		row, err := tx.LoadIntent(t.Context(), intentID)
		if err != nil {
			return err
		}
		command := prepareCommand(t, row, "command-1")
		if _, _, err := tx.StoreCommandOnce(t.Context(), row, command, fixtureNow); err != nil {
			return err
		}
		if tenant, found, err := tx.CompensationTenant(t.Context(), command.ID); err != nil || !found || tenant != row.TenantID {
			t.Errorf("known command: tenant %q found %t, %v", tenant, found, err)
		}
		if tenant, found, err := tx.CompensationTenant(t.Context(), "absent"); err != nil || found || tenant != "" {
			t.Errorf("unknown command: tenant %q found %t, %v", tenant, found, err)
		}
		return nil
	})
}
