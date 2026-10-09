package store

import (
	"context"
	"strings"
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/operators"
)

func TestOperatorStateScopeIsAnEntityAndItsCompositeKeysOnly(t *testing.T) {
	t.Parallel()
	s := openStore(t)
	inTx(t, s, func(tx *Tx) error {
		if err := tx.SaveOperatorState(t.Context(), 0, "m1", operatorState("m1", "m1\x1fa"), testNow); err != nil {
			return err
		}
		return tx.SaveOperatorState(t.Context(), 0, "m10", operatorState("m10"), testNow)
	})
	inTx(t, s, func(tx *Tx) error {
		m1, err := tx.LoadOperatorState(t.Context(), 0, "m1")
		if err != nil || len(m1.OperatorStates["op"]) != 2 {
			t.Fatalf("m1 scope = %v err=%v, want m1 and its composite key but not m10", m1.OperatorStates, err)
		}
		all, err := tx.LoadOperatorState(t.Context(), 0, "")
		if err != nil || len(all.OperatorStates["op"]) != 3 {
			t.Fatalf("partition scope = %v err=%v, want all three keys", all.OperatorStates, err)
		}
		return tx.SaveOperatorState(t.Context(), 0, "m1", operatorState("m1"), testNow)
	})
	inTx(t, s, func(tx *Tx) error {
		all, err := tx.LoadOperatorState(t.Context(), 0, "")
		if err != nil || len(all.OperatorStates["op"]) != 2 {
			t.Fatalf("saving m1 must replace only m1's scope: %v err=%v", all.OperatorStates, err)
		}
		return nil
	})
}

func TestOperatorStateIsScopedToPartitionAndTenant(t *testing.T) {
	t.Parallel()
	s := openStore(t)
	inTx(t, s, func(tx *Tx) error { return tx.SaveOperatorState(t.Context(), 0, "m1", operatorState("m1"), testNow) })
	otherTenant := New(s.db, allowOwner, "epoch", "other", s.deploymentID)
	for name, probe := range map[string]struct {
		store     Store
		partition int
	}{"other partition": {s, 1}, "other tenant": {otherTenant, 0}} {
		inTx(t, probe.store, func(tx *Tx) error {
			state, err := tx.LoadOperatorState(t.Context(), probe.partition, "m1")
			if err != nil || len(state.OperatorStates) != 0 {
				t.Errorf("%s sees operator state %v err=%v", name, state.OperatorStates, err)
			}
			return nil
		})
	}
}

func TestSavingNilOperatorStateLeavesStorageUntouched(t *testing.T) {
	t.Parallel()
	s := openStore(t)
	inTx(t, s, func(tx *Tx) error { return tx.SaveOperatorState(t.Context(), 0, "m1", operatorState("m1"), testNow) })
	inTx(t, s, func(tx *Tx) error { return tx.SaveOperatorState(t.Context(), 0, "m1", nil, testNow) })
	inTx(t, s, func(tx *Tx) error {
		state, err := tx.LoadOperatorState(t.Context(), 0, "m1")
		if err != nil || len(state.OperatorStates["op"]) != 1 {
			t.Fatalf("state = %v err=%v, want the earlier state kept", state.OperatorStates, err)
		}
		return nil
	})
}

func TestStoredOperatorStateRoundTripsItsBlob(t *testing.T) {
	t.Parallel()
	s := openStore(t)
	state := &operators.PartitionState{OperatorStates: map[string]map[string]*operators.OperatorStateBlob{
		"hb": {"m1": {Heartbeat: &operators.HeartbeatState{LastEventID: "evt-9", LastEventTime: &testNow}}},
	}}
	inTx(t, s, func(tx *Tx) error { return tx.SaveOperatorState(t.Context(), 0, "m1", state, testNow) })
	inTx(t, s, func(tx *Tx) error {
		loaded, err := tx.LoadOperatorState(t.Context(), 0, "m1")
		heartbeat := loaded.OperatorStates["hb"]["m1"].Heartbeat
		if err != nil || heartbeat == nil || heartbeat.LastEventID != "evt-9" || !heartbeat.LastEventTime.Equal(testNow) {
			t.Fatalf("loaded = %+v err=%v, want the saved heartbeat", loaded.OperatorStates, err)
		}
		return nil
	})
}

func TestCorruptOperatorStateRefusesTheLoadInEitherScope(t *testing.T) {
	t.Parallel()
	for entityID, want := range map[string]string{"m1": "unmarshal operator state", "": "unmarshal partition operator state"} {
		t.Run("entity "+entityID, func(t *testing.T) {
			t.Parallel()
			s := openStore(t)
			inTx(t, s, func(tx *Tx) error { return tx.SaveOperatorState(t.Context(), 0, "m1", operatorState("m1"), testNow) })
			if _, err := s.db.ExecContext(t.Context(), "UPDATE operator_state SET state_blob = X'5B5D'"); err != nil {
				t.Fatal(err)
			}
			err := s.WithTx(t.Context(), func(tx *Tx) error {
				_, err := tx.LoadOperatorState(t.Context(), 0, entityID)
				return err
			})
			if err == nil || !strings.Contains(err.Error(), want) {
				t.Fatalf("err = %v, want %q", err, want)
			}
		})
	}
}

func TestOperatorStateStatementsNameTheirFailure(t *testing.T) {
	t.Parallel()
	cases := []txFailure{
		{"entity load", "operator_state", func(ctx context.Context, tx *Tx) error { _, err := tx.LoadOperatorState(ctx, 0, "m1"); return err }, "query operator state"},
		{"partition load", "operator_state", func(ctx context.Context, tx *Tx) error { _, err := tx.LoadOperatorState(ctx, 0, ""); return err }, "query partition operator state"},
		{"save", "operator_state", func(ctx context.Context, tx *Tx) error {
			return tx.SaveOperatorState(ctx, 0, "m1", operatorState("m1"), testNow)
		}, "retire prior operator state"},
	}
	checkTxFailures(t, cases)
}
