package domain

import (
	"reflect"
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/spec"
)

func timePolicy() spec.TimePolicy {
	return spec.TimePolicy{MaxOutOfOrderness: "1m", IdleTimeout: "10m", AllowedLateness: "30m", LatePolicy: "correct", ClockSkewTolerance: "5m"}
}

func place(t *testing.T, source string, eventTime, ingestedAt time.Time, previous Checkpoint) TimePlacement {
	t.Helper()
	placement, err := PlaceInTime(EventClock{Source: source, EventTime: eventTime, IngestedAt: ingestedAt}, previous, timePolicy())
	if err != nil {
		t.Fatal(err)
	}
	return placement
}

func TestAnEventAheadOfItsIngestionBeyondTheToleranceNeverMovesTheWatermark(t *testing.T) {
	t.Parallel()
	previous := Checkpoint{Watermark: base, Sources: map[string]SourceClock{"a": {MaxEventTime: base.Add(time.Minute), LastIngestedAt: base}}}
	skewed := place(t, "a", base.Add(24*time.Hour), base.Add(time.Minute), previous)
	if skewed.Disposition != LateClockSkew || !skewed.Clock.Watermark.Equal(base) || !reflect.DeepEqual(skewed.Clock.Sources, previous.Sources) {
		t.Fatalf("skewed placement = %+v, want clock_skew leaving the clock untouched", skewed)
	}
	if within := place(t, "a", base.Add(5*time.Minute), base, previous); within.Disposition != OnTime {
		t.Fatalf("an event exactly at the tolerance = %q, want on time", within.Disposition)
	}
}

func TestThePartitionWatermarkFollowsItsSlowestActiveSource(t *testing.T) {
	t.Parallel()
	first := place(t, "fast", base.Add(10*time.Minute), base.Add(10*time.Minute), Checkpoint{})
	second := place(t, "slow", base.Add(2*time.Minute), base.Add(10*time.Minute), Checkpoint{Watermark: first.Clock.Watermark, Sources: first.Clock.Sources})
	if want := base.Add(9 * time.Minute); !second.Clock.Watermark.Equal(want) {
		t.Fatalf("watermark = %s, want it held at %s: a slower source never pulls it back", second.Clock.Watermark, want)
	}
	third := place(t, "fast", base.Add(12*time.Minute), base.Add(12*time.Minute), Checkpoint{Watermark: second.Clock.Watermark, Sources: second.Clock.Sources})
	if want := base.Add(9 * time.Minute); !third.Clock.Watermark.Equal(want) {
		t.Fatalf("watermark = %s, want %s: the slow source still holds it back", third.Clock.Watermark, want)
	}
	afterIdle := place(t, "fast", base.Add(25*time.Minute), base.Add(25*time.Minute), Checkpoint{Watermark: third.Clock.Watermark, Sources: third.Clock.Sources})
	if want := base.Add(24 * time.Minute); !afterIdle.Clock.Watermark.Equal(want) {
		t.Fatalf("watermark = %s, want %s once the slow source has been idle past the timeout", afterIdle.Clock.Watermark, want)
	}
}

func TestSourceClocksSurviveTheirStoredForm(t *testing.T) {
	t.Parallel()
	sources := map[string]SourceClock{"a": {MaxEventTime: base, LastIngestedAt: base.Add(time.Second)}}
	encoded, err := EncodeSourceClocks(sources)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := DecodeSourceClocks(encoded)
	if err != nil || !reflect.DeepEqual(decoded, sources) {
		t.Fatalf("decoded = %+v err=%v, want %+v", decoded, err, sources)
	}
	if empty, err := EncodeSourceClocks(nil); err != nil || empty != nil {
		t.Fatalf("no sources encode to %q err=%v, want nothing", empty, err)
	}
	if _, err := DecodeSourceClocks([]byte(`{"a":{"max_event_time":"soon"}}`)); err == nil {
		t.Fatal("an unreadable source clock was accepted")
	}
}
