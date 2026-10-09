package store_test

import (
	"testing"
)

func TestCostLimitsReservationsAndSettlement(t *testing.T) {
	t.Parallel()
	persistence, _ := openStore(t)
	tx := persistence.Autocommit()
	ctx, scope := t.Context(), "tenant:t"
	if exists, err := tx.LimitExists(ctx, scope); err != nil || exists {
		t.Fatalf("limit exists before it is written: exists=%v err=%v", exists, err)
	}
	if rows, err := tx.WriteLimit(ctx, scope, "t", 10, false, now); err != nil || rows != 1 {
		t.Fatalf("write rows=%d err=%v", rows, err)
	}
	if maxMicro, kill, err := tx.ReadLimit(ctx, scope); err != nil || maxMicro != 10 || kill {
		t.Fatalf("limit = %d %v %v", maxMicro, kill, err)
	}
	if rows, err := tx.ReserveAvailable(ctx, scope, 11, now); err != nil || rows != 0 {
		t.Fatalf("over-ceiling reservation admitted: rows=%d err=%v", rows, err)
	}
	if rows, err := tx.ReserveAvailable(ctx, scope, 6, now); err != nil || rows != 1 {
		t.Fatalf("reserve rows=%d err=%v", rows, err)
	}
	if rows, err := tx.SettleLimit(ctx, scope, 6, 10, now); err != nil || rows != 1 {
		t.Fatalf("settle rows=%d err=%v", rows, err)
	}
	if _, kill, err := tx.ReadLimit(ctx, scope); err != nil || !kill {
		t.Fatalf("spend reaching the ceiling did not trip the kill switch: kill=%v err=%v", kill, err)
	}
	if rows, err := tx.ReserveAvailable(ctx, scope, 1, now); err != nil || rows != 0 {
		t.Fatalf("a tripped kill switch admitted a reservation: rows=%d err=%v", rows, err)
	}
}

func TestReservationInsertLoadAndSettlementRecord(t *testing.T) {
	t.Parallel()
	persistence, _ := openStore(t)
	tx := persistence.Autocommit()
	ctx := t.Context()
	if _, found, err := tx.LoadReservation(ctx, "ep"); err != nil || found {
		t.Fatalf("reservation found before it is inserted: found=%v err=%v", found, err)
	}
	if err := tx.InsertReservation(ctx, "ep", "t", 7, now); err != nil {
		t.Fatal(err)
	}
	if err := tx.InsertReservation(ctx, "ep", "t", 7, now); err == nil {
		t.Fatal("duplicate reservation accepted")
	}
	reservation, found, err := tx.LoadReservation(ctx, "ep")
	if err != nil || !found || reservation.Reserved != 7 || reservation.TenantID != "t" || reservation.Status != "reserved" {
		t.Fatalf("reservation = %+v found=%v err=%v", reservation, found, err)
	}
	if err := tx.RecordSettlement(ctx, "ep", 5, now); err != nil {
		t.Fatal(err)
	}
	if settled, _, err := tx.LoadReservation(ctx, "ep"); err != nil || settled.Status != "settled" || settled.Actual != 5 {
		t.Fatalf("settled = %+v err=%v", settled, err)
	}
}

func TestCeilingsOfReadsBothScopes(t *testing.T) {
	t.Parallel()
	persistence, _ := openStore(t)
	tx := persistence.Autocommit()
	if _, err := tx.WriteLimit(t.Context(), "tenant:t", "t", 9, true, now); err != nil {
		t.Fatal(err)
	}
	ceilings, err := tx.CeilingsOf(t.Context(), "global", "tenant:t")
	if err != nil || len(ceilings) != 2 {
		t.Fatalf("ceilings = %+v err=%v", ceilings, err)
	}
	found := false
	for _, ceiling := range ceilings {
		found = found || (ceiling.Scope == "tenant:t" && ceiling.MaxMicro == 9 && ceiling.KillSwitch)
	}
	if !found {
		t.Fatalf("tenant ceiling missing: %+v", ceilings)
	}
}
