package domain

import (
	"crypto/sha256"
	"errors"
	"testing"
	"time"
)

func TestRefuseResumeDecidesExpiryBeforeLag(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name     string
		cursor   int64
		maxLag   int64
		retained Retained
		want     error
		action   string
	}{
		{"empty tenant", 0, 0, Retained{}, nil, ""},
		{"resume at oldest", 4, 0, Retained{Oldest: 5, HasOldest: true, NextCursor: 10, HasNextCursor: true}, nil, ""},
		{"before oldest", 3, 0, Retained{Oldest: 5, HasOldest: true, NextCursor: 10, HasNextCursor: true}, ErrCursorExpired, AuditCursorExpired},
		{"negative cursor", -1, 0, Retained{Oldest: 1, HasOldest: true, NextCursor: 3, HasNextCursor: true}, ErrCursorExpired, AuditCursorExpired},
		{"all pruned, behind highwater", 1, 0, Retained{NextCursor: 10, HasNextCursor: true}, ErrCursorExpired, AuditCursorExpired},
		{"all pruned, at highwater", 9, 0, Retained{NextCursor: 10, HasNextCursor: true}, nil, ""},
		{"lag within bound", 5, 5, Retained{Oldest: 1, HasOldest: true, NextCursor: 11, HasNextCursor: true}, nil, ""},
		{"lag beyond bound", 4, 5, Retained{Oldest: 1, HasOldest: true, NextCursor: 11, HasNextCursor: true}, ErrSubscriberTooSlow, AuditSubscriberTooSlow},
		{"lag unbounded", 0, 0, Retained{Oldest: 1, HasOldest: true, NextCursor: 1000, HasNextCursor: true}, nil, ""},
		{"expiry beats lag", 0, 1, Retained{Oldest: 9, HasOldest: true, NextCursor: 100, HasNextCursor: true}, ErrCursorExpired, AuditCursorExpired},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			refusal, refused := RefuseResume(test.cursor, test.maxLag, test.retained)
			if refused != (test.want != nil) || !errors.Is(refusal.Err, test.want) || refusal.Action != test.action {
				t.Fatalf("refusal = %+v refused=%v, want %v/%s", refusal, refused, test.want, test.action)
			}
		})
	}
}

func TestDecodeRecordVerifiesDigestJSONAndEnvelope(t *testing.T) {
	t.Parallel()
	sealed, err := Seal(sampleEvent("a", sealNow), sealNow)
	if err != nil {
		t.Fatal(err)
	}
	record, ok := DecodeRecord("t", 7, sealed.JSON, sealed.SHA)
	if !ok || record.Cursor != 7 || record.TenantID != "t" || record.Event.ID != "a" {
		t.Fatalf("record = %+v ok=%v", record, ok)
	}
	bad := []byte("{bad")
	badSum := sha256.Sum256(bad)
	noDigest := []byte(`{"specversion":"1.0"}`)
	noDigestSum := sha256.Sum256(noDigest)
	for name, row := range map[string]struct{ json, sha []byte }{
		"digest mismatch":  {sealed.JSON, sealed.SHA[:31]},
		"undecodable":      {bad, badSum[:]},
		"invalid envelope": {noDigest, noDigestSum[:]},
	} {
		if _, ok := DecodeRecord("t", 1, row.json, row.sha); ok {
			t.Errorf("%s: poison record decoded", name)
		}
	}
}

func TestPoisonBudgetAndPageLimits(t *testing.T) {
	t.Parallel()
	for attempts, want := range map[int]bool{1: false, 2: false, 3: true, 4: true} {
		if PoisonSpent(attempts) != want {
			t.Errorf("PoisonSpent(%d) = %v, want %v", attempts, !want, want)
		}
	}
	for _, limit := range []int{0, -1, MaxPageLimit + 1} {
		if err := CheckPageLimit(limit); err == nil {
			t.Errorf("limit %d accepted", limit)
		}
	}
	for _, limit := range []int{1, MaxPageLimit} {
		if err := CheckPageLimit(limit); err != nil {
			t.Errorf("limit %d refused: %v", limit, err)
		}
	}
}

func TestPageAdvancesPastDeliveredAndSkippedRecords(t *testing.T) {
	t.Parallel()
	page := Page{NextCursor: 3}
	page.Deliver(Record{Cursor: 4})
	page.Skip(5)
	if page.NextCursor != 5 || len(page.Records) != 1 || page.Skipped != 1 {
		t.Fatalf("page = %+v", page)
	}
}

func TestRetentionRules(t *testing.T) {
	t.Parallel()
	if err := CheckRetention(RetentionFloor - time.Nanosecond); err == nil {
		t.Fatal("retention below the floor accepted")
	}
	if err := CheckRetention(RetentionFloor); err != nil {
		t.Fatalf("retention at the floor refused: %v", err)
	}
	if got := RetentionCutoff(sealNow, RetentionFloor); !got.Equal(sealNow.Add(-RetentionFloor)) {
		t.Fatalf("cutoff = %v", got)
	}
	details, err := AuditDetails(3, 1)
	if err != nil || string(details) != `{"oldest_cursor":1,"requested_cursor":3}` {
		t.Fatalf("details = %s err=%v", details, err)
	}
}
