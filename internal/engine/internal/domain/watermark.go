package domain

import (
	"fmt"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/spec"
)

type Checkpoint struct {
	LastPosition int64
	Watermark    time.Time
	Sources      map[string]SourceClock
}

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

func WatermarkFor(eventTime time.Time, maxOutOfOrderness string, previous time.Time) (time.Time, error) {
	maxLag, err := spec.ParseDuration(maxOutOfOrderness)
	if err != nil {
		return time.Time{}, fmt.Errorf("parse maxOutOfOrderness: %w", err)
	}
	watermark := eventTime.Add(-maxLag)
	if previous.IsZero() {
		return watermark, nil
	}
	return latest(watermark, previous), nil
}

func latest(watermark, previous time.Time) time.Time {
	if watermark.Before(previous) {
		return previous
	}
	return watermark
}

func TimerWatermark(checkpointWatermark, now time.Time) time.Time {
	if checkpointWatermark.IsZero() {
		return now
	}
	return checkpointWatermark
}
