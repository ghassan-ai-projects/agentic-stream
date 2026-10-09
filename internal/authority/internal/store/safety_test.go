package store

import (
	"strings"
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/authority/internal/domain"
)

func TestSafetyEventsAreReadBackAsRecorded(t *testing.T) {
	t.Parallel()
	s, _ := openStore(t)
	event := domain.SafetyEvent{Type: domain.SafetyPhysicalTransition, Target: "fan-01", Details: map[string]any{"source": "s"}, Occurred: testNow}
	work(t, s, func(tx *Tx) error { return tx.AppendSafetyEvent(t.Context(), event) })
	work(t, s, func(tx *Tx) error { return tx.InsertFirstState(t.Context(), deviceState(t, bootA), owner, testNow) })
	work(t, s, func(tx *Tx) error {
		events, err := tx.SafetyEvents(t.Context())
		if err != nil || len(events) != 1 || events[0].Details["source"] != "s" || !events[0].Occurred.Equal(testNow) {
			t.Fatalf("events = %+v, %v", events, err)
		}
		open, err := tx.CountOpenReconciliations(t.Context())
		if err != nil || open != 0 {
			t.Fatalf("open reconciliations = %d, %v", open, err)
		}
		return nil
	})
}

func TestTheAuthorityAuditLogIsCounted(t *testing.T) {
	t.Parallel()
	s, _ := openStore(t)
	count := func() uint64 {
		var events uint64
		work(t, s, func(tx *Tx) (err error) {
			events, err = tx.CountAuthorityEvents(t.Context())
			return err
		})
		return events
	}
	if events := count(); events != 0 {
		t.Fatalf("events in an empty log = %d", events)
	}
	work(t, s, func(tx *Tx) error { return tx.AppendAuthorityEvent(t.Context(), domain.ReleaseEvent(claim, testNow)) })
	work(t, s, func(tx *Tx) error { return tx.AppendAuthorityEvent(t.Context(), domain.ReleaseEvent(claim, testNow)) })
	if events := count(); events != 2 {
		t.Fatalf("events after two appends = %d, want 2", events)
	}
}

func TestSafetyEvidenceThatNoLongerVerifiesIsRefusedWhicheverPartChanged(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		change string
		want   string
	}{
		{"the details were rewritten", `UPDATE device_safety_events SET details_json = CAST('{"source":"x"}' AS BLOB)`, "verify safety event"},
		{"the digest was cleared", `UPDATE device_safety_events SET details_sha256 = zeroblob(32)`, "verify safety event"},
		{"the time was corrupted", `UPDATE device_safety_events SET occurred_at = 'yesterday'`, "parse safety event time"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			s, db := openStore(t)
			event := domain.SafetyEvent{Type: domain.SafetyPhysicalTransition, Target: "fan-01", Details: map[string]any{"source": "s"}, Occurred: testNow}
			work(t, s, func(tx *Tx) error { return tx.AppendSafetyEvent(t.Context(), event) })
			if _, err := db.ExecContext(t.Context(), test.change); err != nil {
				t.Fatal(err)
			}
			err := s.InTx(t.Context(), func(tx *Tx) error {
				_, err := tx.SafetyEvents(t.Context())
				return err
			})
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("SafetyEvents = %v, want a refusal mentioning %q", err, test.want)
			}
		})
	}
}
