package domain

import (
	"strings"
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/spec"
)

var base = time.Date(2026, 10, 6, 12, 0, 0, 0, time.UTC)

func TestWatermarkTrailsEventTimeAndNeverMovesBackwards(t *testing.T) {
	t.Parallel()
	got, err := WatermarkFor(base, "5m", time.Time{})
	if err != nil || !got.Equal(base.Add(-5*time.Minute)) {
		t.Fatalf("first watermark = %v, %v", got, err)
	}
	later := base.Add(-time.Minute)
	if got, err := WatermarkFor(base, "5m", later); err != nil || !got.Equal(base.Add(-time.Minute)) {
		t.Fatalf("a watermark behind the previous must hold: %v, %v", got, err)
	}
	earlier := base.Add(-time.Hour)
	if got, err := WatermarkFor(base, "5m", earlier); err != nil || !got.Equal(base.Add(-5*time.Minute)) {
		t.Fatalf("a watermark ahead of the previous must advance: %v, %v", got, err)
	}
}

func TestWatermarkRefusesAnUnparseableLag(t *testing.T) {
	t.Parallel()
	if _, err := WatermarkFor(base, "soon", base); err == nil || !strings.Contains(err.Error(), "parse maxOutOfOrderness") {
		t.Fatalf("lag err = %v", err)
	}
}

func TestTimerWatermarkFallsBackToNowBeforeTheFirstRecord(t *testing.T) {
	t.Parallel()
	if got := TimerWatermark(time.Time{}, base); !got.Equal(base) {
		t.Fatalf("no checkpoint: %v", got)
	}
	checkpoint := base.Add(-time.Hour)
	if got := TimerWatermark(checkpoint, base); !got.Equal(checkpoint) {
		t.Fatalf("checkpoint: %v", got)
	}
}

func TestSchemaValidationIsRequiredOnlyWhenEveryInputDeclaresASchema(t *testing.T) {
	t.Parallel()
	declared := spec.Input{SchemaRef: "motor.v1"}
	if RequiresSchemaValidation(nil) || RequiresSchemaValidation([]spec.Input{declared, {}}) || RequiresSchemaValidation([]spec.Input{{}}) {
		t.Fatal("schema validation must stay optional")
	}
	if !RequiresSchemaValidation([]spec.Input{declared, declared}) {
		t.Fatal("every input declares a schema, so validation is required")
	}
}
