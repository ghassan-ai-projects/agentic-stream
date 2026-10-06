package domain

import (
	"fmt"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/spec"
)

// Checkpoint is the engine's progress in one partition: the last applied log
// position and the watermark it reached ("" before the first record).
type Checkpoint struct {
	LastPosition int64
	Watermark    string
}

// RequiresSchemaValidation reports whether every declared input names a schema,
// which makes event schema validation mandatory. An input without a schema, or
// no inputs at all, leaves it optional.
func RequiresSchemaValidation(inputs []spec.Input) bool {
	if len(inputs) == 0 {
		return false
	}
	for _, input := range inputs {
		if input.SchemaRef == "" {
			return false
		}
	}
	return true
}

// WatermarkFor derives a record's watermark as its event time minus the
// maximum out-of-orderness, never moving before the previous watermark.
func WatermarkFor(eventTime time.Time, maxOutOfOrderness, previous string) (time.Time, error) {
	maxLag, err := spec.ParseDuration(maxOutOfOrderness)
	if err != nil {
		return time.Time{}, fmt.Errorf("parse maxOutOfOrderness: %w", err)
	}
	watermark := eventTime.Add(-maxLag)
	if previous == "" {
		return watermark, nil
	}
	prior, err := time.Parse(time.RFC3339Nano, previous)
	if err != nil {
		return time.Time{}, fmt.Errorf("parse prev watermark: %w", err)
	}
	return latest(watermark, prior), nil
}

// latest keeps the watermark monotonic: it never moves before previous.
func latest(watermark, previous time.Time) time.Time {
	if watermark.Before(previous) {
		return previous
	}
	return watermark
}

// TimerWatermark is the watermark due timers fire under: the partition's
// checkpoint watermark, or now before the first record.
func TimerWatermark(checkpointWatermark string, now time.Time) (time.Time, error) {
	if checkpointWatermark == "" {
		return now, nil
	}
	watermark, err := time.Parse(time.RFC3339Nano, checkpointWatermark)
	if err != nil {
		return time.Time{}, fmt.Errorf("parse timer watermark: %w", err)
	}
	return watermark, nil
}
