package store

import (
	"context"
	"strings"
	"testing"
	"time"

	domain "github.com/ghassan-ai-projects/agentic-stream/internal/eventlog/internal/domain"
)

func quarantinePayload(t *testing.T, eventID string, version int) domain.QuarantinePayload {
	t.Helper()
	payload, err := domain.NewQuarantinePayload(map[string]any{"id": eventID, "type": "sensor.temperature", "v": version})
	if err != nil {
		t.Fatal(err)
	}
	return payload
}

func upsert(t *testing.T, st Store, payload domain.QuarantinePayload, at time.Time) int64 {
	t.Helper()
	var rows int64
	inUnit(t, st, func(ctx context.Context, u *Unit) error {
		var err error
		rows, err = u.UpsertQuarantine(ctx, payload, "tenant", "malformed_json", at)
		return err
	})
	return rows
}

func quarantineStatus(t *testing.T, st Store, eventID string) string {
	t.Helper()
	var status string
	inUnit(t, st, func(ctx context.Context, u *Unit) error {
		var err error
		status, err = u.QuarantineStatus(ctx, "tenant", eventID)
		return err
	})
	return status
}

func TestUpsertQuarantineCountsRepeatedDeliveriesOfTheSamePayload(t *testing.T) {
	t.Parallel()
	st := newStore(t)
	payload := quarantinePayload(t, "bad-1", 1)
	for range 3 {
		if rows := upsert(t, st, payload, storedAt); rows != 1 {
			t.Fatalf("rows affected = %d, want 1", rows)
		}
	}
	records, err := st.Quarantined(t.Context(), "tenant")
	if err != nil || len(records) != 1 || records[0].AttemptCount != 3 || records[0].Status != "quarantined" || records[0].ReasonCode != "malformed_json" {
		t.Fatalf("records = %+v err=%v, want one quarantined record with 3 attempts", records, err)
	}
}

func TestUpsertQuarantineRefusesADifferentPayloadUnderTheSameEventID(t *testing.T) {
	t.Parallel()
	st := newStore(t)
	upsert(t, st, quarantinePayload(t, "bad-1", 1), storedAt)
	if rows := upsert(t, st, quarantinePayload(t, "bad-1", 2), storedAt.Add(time.Minute)); rows != 0 {
		t.Fatalf("rows affected = %d, want 0 for a conflicting payload", rows)
	}
	var digest []byte
	inUnit(t, st, func(ctx context.Context, u *Unit) error {
		var err error
		digest, err = u.QuarantineDigest(ctx, "tenant", "bad-1")
		return err
	})
	if string(digest) != string(quarantinePayload(t, "bad-1", 1).Digest) {
		t.Fatal("the stored digest changed after a conflicting delivery")
	}
}

func TestQuarantineRejectsTheEleventhDeliveryOfThePayload(t *testing.T) {
	t.Parallel()
	st := newStore(t)
	payload := quarantinePayload(t, "bad-1", 1)
	for delivery := 1; delivery <= 10; delivery++ {
		upsert(t, st, payload, storedAt)
		if status := quarantineStatus(t, st, "bad-1"); status != "quarantined" {
			t.Fatalf("status after delivery %d = %q, want quarantined", delivery, status)
		}
	}
	upsert(t, st, payload, storedAt)
	upsert(t, st, payload, storedAt)
	records, err := st.Quarantined(t.Context(), "tenant")
	if err != nil || records[0].Status != "rejected" || records[0].AttemptCount != 10 {
		t.Fatalf("records = %+v err=%v, want rejected at the bound of 10 attempts", records, err)
	}
}

func TestQuarantineConflictAndOverflowAreRecordedOnce(t *testing.T) {
	t.Parallel()
	st := newStore(t)
	payload := quarantinePayload(t, "bad-1", 1)
	upsert(t, st, payload, storedAt)
	inUnit(t, st, func(ctx context.Context, u *Unit) error {
		return u.RejectQuarantineConflict(ctx, "tenant", "bad-1", storedAt.Add(time.Minute))
	})
	records, err := st.Quarantined(t.Context(), "tenant")
	if err != nil || records[0].Status != "rejected" || records[0].ReasonCode != "event_id_hash_conflict" {
		t.Fatalf("records = %+v err=%v, want rejected with event_id_hash_conflict", records, err)
	}
	for range 2 {
		inUnit(t, st, func(ctx context.Context, u *Unit) error {
			return u.InsertOverflowGap(ctx, payload.OverflowGapID(), "tenant", storedAt)
		})
	}
	var gaps int
	if err := st.DB.QueryRowContext(t.Context(), "SELECT COUNT(*) FROM event_gaps WHERE gap_id = ? AND reason_code = 'quarantine_retry_exhausted'", payload.OverflowGapID()).Scan(&gaps); err != nil || gaps != 1 {
		t.Fatalf("gaps = %d err=%v, want exactly one gap for the quarantine", gaps, err)
	}
}

func TestQuarantineLookupsReportAbsence(t *testing.T) {
	t.Parallel()
	st := newStore(t)
	inUnit(t, st, func(ctx context.Context, u *Unit) error {
		if digest, err := u.QuarantineDigest(ctx, "tenant", "missing"); err != nil || digest != nil {
			t.Fatalf("digest = %x err=%v, want none", digest, err)
		}
		if _, err := u.QuarantineStatus(ctx, "tenant", "missing"); err == nil || !strings.Contains(err.Error(), "read quarantine status") {
			t.Fatalf("status err = %v, want read quarantine status", err)
		}
		return nil
	})
}

func TestReleaseAllowsRedriveOnceAndOnlyForQuarantinedRecords(t *testing.T) {
	t.Parallel()
	st := newStore(t)
	upsert(t, st, quarantinePayload(t, "bad-1", 1), storedAt)
	inUnit(t, st, func(ctx context.Context, u *Unit) error {
		if _, err := u.ReleasedEnvelope(ctx, "tenant", "bad-1"); err == nil || !strings.Contains(err.Error(), "is not released") {
			t.Fatalf("unreleased envelope err = %v, want is not released", err)
		}
		return u.ReleaseQuarantined(ctx, "tenant", "bad-1", storedAt)
	})
	inUnit(t, st, func(ctx context.Context, u *Unit) error {
		if err := u.ReleaseQuarantined(ctx, "tenant", "bad-1", storedAt); err == nil || !strings.Contains(err.Error(), "not available for release") {
			t.Fatalf("double release err = %v", err)
		}
		env, err := u.ReleasedEnvelope(ctx, "tenant", "bad-1")
		if err != nil || env.ID != "bad-1" || env.Type != "sensor.temperature" {
			t.Fatalf("released envelope = %+v err=%v", env, err)
		}
		return u.MarkRedriven(ctx, "tenant", "bad-1", storedAt)
	})
	inUnit(t, st, func(ctx context.Context, u *Unit) error {
		if _, err := u.ReleasedEnvelope(ctx, "tenant", "bad-1"); err == nil || !strings.Contains(err.Error(), "is not released") {
			t.Fatalf("redriven envelope err = %v, want is not released", err)
		}
		return nil
	})
	records, err := st.Quarantined(t.Context(), "tenant")
	if err != nil || records[0].Status != "redriven" {
		t.Fatalf("records = %+v err=%v, want status redriven", records, err)
	}
}

func TestReleaseRefusesRejectedAndUnknownRecords(t *testing.T) {
	t.Parallel()
	st := newStore(t)
	upsert(t, st, quarantinePayload(t, "bad-1", 1), storedAt)
	inUnit(t, st, func(ctx context.Context, u *Unit) error {
		return u.RejectQuarantineConflict(ctx, "tenant", "bad-1", storedAt)
	})
	inUnit(t, st, func(ctx context.Context, u *Unit) error {
		for _, eventID := range []string{"bad-1", "missing"} {
			if err := u.ReleaseQuarantined(ctx, "tenant", eventID, storedAt); err == nil || !strings.Contains(err.Error(), "not available for release") {
				t.Errorf("release of %q err = %v, want not available for release", eventID, err)
			}
		}
		if _, err := u.ReleasedEnvelope(ctx, "tenant", "missing"); err == nil || !strings.Contains(err.Error(), "load released quarantine") {
			t.Errorf("unknown envelope err = %v, want load released quarantine", err)
		}
		return nil
	})
}

func TestReleasedEnvelopeRefusesAnUndecodablePayload(t *testing.T) {
	t.Parallel()
	st := newStore(t)
	upsert(t, st, quarantinePayload(t, "bad-1", 1), storedAt)
	inUnit(t, st, func(ctx context.Context, u *Unit) error {
		return u.ReleaseQuarantined(ctx, "tenant", "bad-1", storedAt)
	})
	if _, err := st.DB.ExecContext(t.Context(), `UPDATE event_quarantine SET payload_json = X'5B5D'`); err != nil {
		t.Fatal(err)
	}
	inUnit(t, st, func(ctx context.Context, u *Unit) error {
		if _, err := u.ReleasedEnvelope(ctx, "tenant", "bad-1"); err == nil || !strings.Contains(err.Error(), "decode released envelope") {
			t.Fatalf("err = %v, want decode released envelope", err)
		}
		return nil
	})
}

func TestQuarantinedListsTheTenantsRecordsNewestFirstWithinTheBound(t *testing.T) {
	t.Parallel()
	st := newStore(t)
	upsert(t, st, quarantinePayload(t, "oldest", 1), storedAt)
	upsert(t, st, quarantinePayload(t, "newest", 1), storedAt.Add(time.Hour))
	inUnit(t, st, func(ctx context.Context, u *Unit) error {
		_, err := u.UpsertQuarantine(ctx, quarantinePayload(t, "elsewhere", 1), "other", "reason", storedAt)
		return err
	})
	records, err := st.Quarantined(t.Context(), "tenant")
	if err != nil || len(records) != 2 || records[0].EventID != "newest" || records[1].EventID != "oldest" {
		t.Fatalf("records = %+v err=%v, want newest then oldest, without the other tenant", records, err)
	}
	if records[0].FirstSeenAt == "" || records[0].LastSeenAt == "" || records[0].EventType != "sensor.temperature" {
		t.Fatalf("record = %+v, want first/last seen and event type", records[0])
	}

	_, err = st.DB.ExecContext(t.Context(), `WITH RECURSIVE n(i) AS (SELECT 1 UNION ALL SELECT i + 1 FROM n WHERE i < ?)
		INSERT INTO event_quarantine (quarantine_id, tenant_id, event_id, event_type, schema_version, source, reason_code, payload_json, payload_sha256, status, first_seen_at, last_seen_at)
		SELECT 'q-bulk-' || i, 'tenant', 'bulk-' || i, 'sensor.temperature', '1.0', 'test', 'malformed_json', X'7B7D', zeroblob(32), 'quarantined', ?, ?
		FROM n`, quarantineListLimit, "2026-08-12T14:00:00Z", "2026-08-12T14:00:00Z")
	if err != nil {
		t.Fatal(err)
	}
	bounded, err := st.Quarantined(t.Context(), "tenant")
	if err != nil || len(bounded) != quarantineListLimit {
		t.Fatalf("listed %d records err=%v, want the bound %d", len(bounded), err, quarantineListLimit)
	}
}

func TestQuarantineStatementsNameTheirFailure(t *testing.T) {
	t.Parallel()
	payload := quarantinePayload(t, "bad-1", 1)
	cases := []unitFailure{
		{"digest", "event_quarantine", func(ctx context.Context, u *Unit) error {
			_, err := u.QuarantineDigest(ctx, "tenant", "bad-1")
			return err
		}, "load existing quarantine"},
		{"upsert", "event_quarantine", func(ctx context.Context, u *Unit) error {
			_, err := u.UpsertQuarantine(ctx, payload, "tenant", "reason", storedAt)
			return err
		}, "persist event quarantine"},
		{"conflict", "event_quarantine", func(ctx context.Context, u *Unit) error {
			return u.RejectQuarantineConflict(ctx, "tenant", "bad-1", storedAt)
		}, "record quarantine hash conflict"},
		{"gap", "event_gaps", func(ctx context.Context, u *Unit) error {
			return u.InsertOverflowGap(ctx, "gap", "tenant", storedAt)
		}, "record quarantine overflow gap"},
		{"release", "event_quarantine", func(ctx context.Context, u *Unit) error {
			return u.ReleaseQuarantined(ctx, "tenant", "bad-1", storedAt)
		}, "release quarantined event"},
		{"redrive mark", "event_quarantine", func(ctx context.Context, u *Unit) error {
			return u.MarkRedriven(ctx, "tenant", "bad-1", storedAt)
		}, "mark quarantine redriven"},
	}
	checkUnitFailures(t, cases)
	if _, err := newStoreWithoutTable(t, "event_quarantine").Quarantined(t.Context(), "tenant"); err == nil || !strings.Contains(err.Error(), "list quarantine") {
		t.Fatalf("list err = %v, want list quarantine", err)
	}
}

func newStoreWithoutTable(t *testing.T, table string) Store {
	t.Helper()
	st := newStore(t)
	if _, err := st.DB.ExecContext(t.Context(), "DROP TABLE "+table); err != nil {
		t.Fatal(err)
	}
	return st
}
