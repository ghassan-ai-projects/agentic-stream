package store

import (
	"strings"
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/authority/internal/domain"
)

func latestOpening(t *testing.T, s *Store, device domain.DeviceBoot) (domain.Opening, bool, error) {
	t.Helper()
	var opening domain.Opening
	var opened bool
	err := s.InTx(t.Context(), func(tx *Tx) (err error) {
		opening, opened, err = tx.LatestOpening(t.Context(), device)
		return err
	})
	return opening, opened, err
}

func TestTheLatestOpeningOfABootIsReadBackWithItsCause(t *testing.T) {
	t.Parallel()
	s, db := openStore(t)
	if _, opened, err := latestOpening(t, s, bootA); err != nil || opened {
		t.Fatalf("an unopened boot = %v, %v; want no opening", opened, err)
	}
	reboot := domain.RebootEvent(domain.StateObservation{State: domain.DeviceState{Device: bootA}, Owner: owner, PreviousBootID: "boot-0"}, testNow)
	unknown := domain.OpeningEvent(domain.ReconciliationOpening{Device: bootA, Owner: owner, Reason: "outcome unknown"}, testNow.Add(time.Second))
	work(t, s, func(tx *Tx) error { return tx.AppendAuthorityEvent(t.Context(), reboot) })
	if opening, opened, err := latestOpening(t, s, bootA); err != nil || !opened || !opening.Reboot || !opening.At.Equal(testNow) {
		t.Fatalf("after a reboot = %+v, %v, %v; want the reboot at %s", opening, opened, err, testNow)
	}
	work(t, s, func(tx *Tx) error { return tx.AppendAuthorityEvent(t.Context(), unknown) })
	if opening, _, err := latestOpening(t, s, bootA); err != nil || opening.Reboot || !opening.At.Equal(testNow.Add(time.Second)) {
		t.Fatalf("after an unknown outcome = %+v, %v; want the later opening", opening, err)
	}
	if _, err := db.ExecContext(t.Context(), `UPDATE device_authority_events SET occurred_at = 'yesterday'`); err != nil {
		t.Fatal(err)
	}
	if _, _, err := latestOpening(t, s, bootA); err == nil || !strings.Contains(err.Error(), "opening time") {
		t.Fatalf("an unreadable opening time = %v, want the refusal", err)
	}
}
