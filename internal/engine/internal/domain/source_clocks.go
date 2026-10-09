package domain

import (
	"encoding/json"
	"fmt"
	"maps"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/kernel"
	"github.com/ghassan-ai-projects/agentic-stream/internal/spec"
)

const LateClockSkew LateDisposition = "clock_skew"

type SourceClock struct {
	MaxEventTime   time.Time
	LastIngestedAt time.Time
}

type EventClock struct {
	Source                string
	EventTime, IngestedAt time.Time
}

type PartitionClock struct {
	Watermark time.Time
	Sources   map[string]SourceClock
}

type TimePlacement struct {
	Clock       PartitionClock
	Disposition LateDisposition
}

func PlaceInTime(event EventClock, previous Checkpoint, policy spec.TimePolicy) (TimePlacement, error) {
	unchanged := PartitionClock{Watermark: previous.Watermark, Sources: previous.Sources}
	skewed, err := beyondClockSkew(event, policy.ClockSkewTolerance)
	if err != nil || skewed {
		return TimePlacement{Clock: unchanged, Disposition: LateClockSkew}, err
	}
	lateness, err := ClassifyLateness(event.EventTime, previous.Watermark, policy)
	if err != nil {
		return TimePlacement{}, err
	}
	sources := advanceSource(previous.Sources, event)
	watermark, err := partitionWatermark(sources, event.IngestedAt, policy, previous.Watermark)
	if err != nil {
		return TimePlacement{}, err
	}
	return TimePlacement{Clock: PartitionClock{Watermark: watermark, Sources: sources}, Disposition: lateness}, nil
}

func beyondClockSkew(event EventClock, tolerance string) (bool, error) {
	if tolerance == "" {
		return false, nil
	}
	allowed, err := spec.ParseDuration(tolerance)
	if err != nil {
		return false, fmt.Errorf("parse clockSkewTolerance: %w", err)
	}
	return event.EventTime.After(event.IngestedAt.Add(allowed)), nil
}

func advanceSource(previous map[string]SourceClock, event EventClock) map[string]SourceClock {
	sources := maps.Clone(previous)
	if sources == nil {
		sources = make(map[string]SourceClock, 1)
	}
	clock := sources[event.Source]
	clock.MaxEventTime = later(clock.MaxEventTime, event.EventTime)
	clock.LastIngestedAt = later(clock.LastIngestedAt, event.IngestedAt)
	sources[event.Source] = clock
	return sources
}

func partitionWatermark(sources map[string]SourceClock, now time.Time, policy spec.TimePolicy, previous time.Time) (time.Time, error) {
	idle, err := optionalDuration(policy.IdleTimeout)
	if err != nil {
		return time.Time{}, fmt.Errorf("parse idleTimeout: %w", err)
	}
	return WatermarkFor(slowestActiveSource(sources, now, idle), policy.MaxOutOfOrderness, previous)
}

func slowestActiveSource(sources map[string]SourceClock, now time.Time, idle time.Duration) time.Time {
	var slowest time.Time
	for _, clock := range sources {
		active := idle == 0 || !clock.LastIngestedAt.Before(now.Add(-idle))
		if active && (slowest.IsZero() || clock.MaxEventTime.Before(slowest)) {
			slowest = clock.MaxEventTime
		}
	}
	return slowest
}

func optionalDuration(text string) (time.Duration, error) {
	if text == "" {
		return 0, nil
	}
	duration, err := spec.ParseDuration(text)
	if err != nil {
		return 0, fmt.Errorf("parse duration: %w", err)
	}
	return duration, nil
}

func later(a, b time.Time) time.Time {
	if b.After(a) {
		return b
	}
	return a
}

type storedSourceClock struct {
	MaxEventTime   string `json:"max_event_time"`
	LastIngestedAt string `json:"last_ingested_at"`
}

func EncodeSourceClocks(sources map[string]SourceClock) ([]byte, error) {
	if len(sources) == 0 {
		return nil, nil
	}
	stored := make(map[string]storedSourceClock, len(sources))
	for source, clock := range sources {
		stored[source] = storedSourceClock{MaxEventTime: kernel.FormatTime(clock.MaxEventTime), LastIngestedAt: kernel.FormatTime(clock.LastIngestedAt)}
	}
	encoded, err := json.Marshal(stored)
	if err != nil {
		return nil, fmt.Errorf("encode source clocks: %w", err)
	}
	return encoded, nil
}

func DecodeSourceClocks(encoded []byte) (map[string]SourceClock, error) {
	if len(encoded) == 0 {
		return nil, nil
	}
	var stored map[string]storedSourceClock
	if err := json.Unmarshal(encoded, &stored); err != nil {
		return nil, fmt.Errorf("decode source clocks: %w", err)
	}
	return decodeSourceClockMap(stored)
}

func decodeSourceClockMap(stored map[string]storedSourceClock) (map[string]SourceClock, error) {
	sources := make(map[string]SourceClock, len(stored))
	for source, clock := range stored {
		decoded, err := decodeSourceClock(clock)
		if err != nil {
			return nil, fmt.Errorf("decode clock of source %s: %w", source, err)
		}
		sources[source] = decoded
	}
	return sources, nil
}

func decodeSourceClock(stored storedSourceClock) (SourceClock, error) {
	maxEventTime, err := kernel.ParseTime(stored.MaxEventTime)
	if err != nil {
		return SourceClock{}, fmt.Errorf("max event time: %w", err)
	}
	lastIngestedAt, err := kernel.ParseTime(stored.LastIngestedAt)
	if err != nil {
		return SourceClock{}, fmt.Errorf("last ingested time: %w", err)
	}
	return SourceClock{MaxEventTime: maxEventTime, LastIngestedAt: lastIngestedAt}, nil
}
