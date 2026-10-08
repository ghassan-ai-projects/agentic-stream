package store

import "testing"

func TestWatchReadsTheConditionAndItsFires(t *testing.T) {
	t.Parallel()
	s := openStore(t)
	if _, found, err := Reader(s.db).Watch(t.Context(), "tenant", "w-1"); err != nil || found {
		t.Fatalf("an uninstalled watch was found: found=%t err=%v", found, err)
	}
	inTx(t, s, func(tx *Tx) error { return tx.InsertCondition(t.Context(), "w-1", condition(), testNow) })
	inTx(t, s, func(tx *Tx) error {
		_, err := tx.RecordFire(t.Context(), "w-1", "evt-1", testNow)
		return err
	})
	watch, found, err := Reader(s.db).Watch(t.Context(), "tenant", "w-1")
	if err != nil || !found || watch.MaxFires != 2 || len(watch.Fires) != 1 || watch.Fires[0].EventID != "evt-1" {
		t.Fatalf("watch = %+v found=%t err=%v", watch, found, err)
	}
}
