package notify_test

import (
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/notify"
)

func TestAppendDuplicatesAndConflictsDoNotConsumeCursors(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	db, outbox := openOutbox(t)
	original := testEvent("same-id", baseTime)
	for range 2 {
		if cursor, err := appendEvent(ctx, db, original, baseTime); err != nil || cursor != 1 {
			t.Fatalf("duplicate cursor=%d err=%v", cursor, err)
		}
	}
	conflict := original
	conflict.Data = map[string]any{"version": 2}
	var err error
	if conflict.EnvelopeDigest, err = conflict.ComputeEnvelopeDigest(); err != nil {
		t.Fatal(err)
	}
	if _, err := appendEvent(ctx, db, conflict, baseTime); err == nil {
		t.Fatal("conflicting payload accepted")
	}
	if cursor, err := appendEvent(ctx, db, testEvent("next-id", baseTime), baseTime); err != nil || cursor != 2 {
		t.Fatalf("next cursor=%d err=%v", cursor, err)
	}
	page, err := outbox.ReadPage(ctx, notify.PageRequest{TenantID: "tenant", Limit: 10}, baseTime)
	if err != nil || len(page.Records) != 2 || page.Records[0].Event.EnvelopeDigest != original.EnvelopeDigest {
		t.Fatalf("stored notifications changed: page=%+v err=%v", page, err)
	}
}
