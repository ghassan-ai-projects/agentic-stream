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
	got, err := WatermarkFor(base, "5m", "")
	if err != nil || !got.Equal(base.Add(-5*time.Minute)) {
		t.Fatalf("first watermark = %v, %v", got, err)
	}
	later := base.Add(-time.Minute).Format(time.RFC3339Nano)
	if got, err := WatermarkFor(base, "5m", later); err != nil || !got.Equal(base.Add(-time.Minute)) {
		t.Fatalf("a watermark behind the previous must hold: %v, %v", got, err)
	}
	earlier := base.Add(-time.Hour).Format(time.RFC3339Nano)
	if got, err := WatermarkFor(base, "5m", earlier); err != nil || !got.Equal(base.Add(-5*time.Minute)) {
		t.Fatalf("a watermark ahead of the previous must advance: %v, %v", got, err)
	}
}

func TestWatermarkRefusesUnparseableInputsInOrder(t *testing.T) {
	t.Parallel()
	if _, err := WatermarkFor(base, "soon", "also bad"); err == nil || !strings.Contains(err.Error(), "parse maxOutOfOrderness") {
		t.Fatalf("lag must be parsed first: %v", err)
	}
	if _, err := WatermarkFor(base, "5m", "bad"); err == nil || !strings.Contains(err.Error(), "parse prev watermark") {
		t.Fatalf("previous watermark err = %v", err)
	}
}

func TestTimerWatermarkFallsBackToNowBeforeTheFirstRecord(t *testing.T) {
	t.Parallel()
	if got, err := TimerWatermark("", base); err != nil || !got.Equal(base) {
		t.Fatalf("no checkpoint: %v, %v", got, err)
	}
	checkpoint := base.Add(-time.Hour)
	if got, err := TimerWatermark(checkpoint.Format(time.RFC3339Nano), base); err != nil || !got.Equal(checkpoint) {
		t.Fatalf("checkpoint: %v, %v", got, err)
	}
	if _, err := TimerWatermark("bad", base); err == nil || !strings.Contains(err.Error(), "parse timer watermark") {
		t.Fatalf("err = %v", err)
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
