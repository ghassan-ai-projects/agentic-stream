package domain

import (
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/spec"
)

func TestLateEventsAreClassifiedByPolicyAndAllowedLateness(t *testing.T) {
	t.Parallel()
	watermark := time.Date(2026, 1, 1, 10, 30, 0, 0, time.UTC)
	for _, tc := range []struct {
		name, policy string
		eventTime    time.Time
		want         LateDisposition
	}{
		{"at the watermark is on time", "drop_with_audit", watermark, OnTime},
		{"after the watermark is on time", "history_only", watermark.Add(time.Minute), OnTime},
		{"correct within allowed lateness", "correct", watermark.Add(-5 * time.Minute), LateCorrected},
		{"correct and reconsider within allowed lateness", "correct_and_reconsider", watermark.Add(-5 * time.Minute), LateCorrected},
		{"history only within allowed lateness", "history_only", watermark.Add(-5 * time.Minute), LateHistoryOnly},
		{"drop with audit within allowed lateness", "drop_with_audit", watermark.Add(-5 * time.Minute), LateDropped},
		{"correct beyond allowed lateness", "correct", watermark.Add(-40 * time.Minute), LateBeyondAllowedLateness},
		{"history only beyond allowed lateness", "history_only", watermark.Add(-40 * time.Minute), LateBeyondAllowedLateness},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := ClassifyLateness(tc.eventTime, watermark, spec.TimePolicy{AllowedLateness: "30m", LatePolicy: tc.policy})
			if err != nil || got != tc.want {
				t.Fatalf("disposition = %q err=%v, want %q", got, err, tc.want)
			}
		})
	}
}

func TestNoEventIsLateBeforeThePartitionHasAWatermark(t *testing.T) {
	t.Parallel()
	got, err := ClassifyLateness(time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC), time.Time{}, spec.TimePolicy{LatePolicy: "drop_with_audit"})
	if err != nil || got != OnTime {
		t.Fatalf("disposition = %q err=%v, want on time", got, err)
	}
}

func TestWithoutAllowedLatenessEveryLateEventIsBeyondIt(t *testing.T) {
	t.Parallel()
	watermark := time.Date(2026, 1, 1, 10, 30, 0, 0, time.UTC)
	got, err := ClassifyLateness(watermark.Add(-time.Second), watermark, spec.TimePolicy{LatePolicy: "correct"})
	if err != nil || got != LateBeyondAllowedLateness {
		t.Fatalf("disposition = %q err=%v, want beyond allowed lateness", got, err)
	}
}

func TestAnUnreadableAllowedLatenessIsRefused(t *testing.T) {
	t.Parallel()
	watermark := time.Date(2026, 1, 1, 10, 30, 0, 0, time.UTC)
	if _, err := ClassifyLateness(watermark.Add(-time.Second), watermark, spec.TimePolicy{AllowedLateness: "soon"}); err == nil {
		t.Fatal("an unreadable allowedLateness was accepted")
	}
}

func TestOnlyOnTimeAndCorrectingEventsChangeState(t *testing.T) {
	t.Parallel()
	for disposition, want := range map[LateDisposition]bool{OnTime: true, LateCorrected: true, LateHistoryOnly: false, LateDropped: false, LateBeyondAllowedLateness: false} {
		if disposition.ChangesState() != want {
			t.Fatalf("%q changes state = %v, want %v", disposition, !want, want)
		}
	}
}
