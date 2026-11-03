package composition_test

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/actionport"
	"github.com/ghassan-ai-projects/agentic-stream/internal/device"
	"github.com/ghassan-ai-projects/agentic-stream/internal/executor/fixture"
	"github.com/ghassan-ai-projects/agentic-stream/internal/kernel"
	"github.com/ghassan-ai-projects/agentic-stream/internal/runtime"
	"github.com/ghassan-ai-projects/agentic-stream/internal/sources"
	"github.com/ghassan-ai-projects/agentic-stream/internal/spec"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage/storagetest"
)

const (
	pumpSpecPath = "../../../../examples/pump-station/pump-station.situation.yaml"
	pumpEntity   = "site-a:pump-3"
)

var pumpWorldStart = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

type pumpMinute struct{ vibration, axial, current float64 }

func TestAPumpStationWatchOfTheSpecsPresetFiresOnceTheVibrationMeanReachesItsThreshold(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	db := storagetest.OpenTemp(t)
	compiled, err := spec.CompileFile(ctx, pumpSpecPath)
	if err != nil {
		t.Fatal(err)
	}
	pipeline, err := runtime.NewPipeline(ctx, runtime.PipelineConfig{DB: db, Spec: compiled, TenantID: "default", Clock: sources.Physical(),
		IDGenerator: sources.Deterministic(), Executor: fixture.New(), Effector: device.NewSimulatedEffector()})
	if err != nil {
		t.Fatal(err)
	}
	runPumpMinutes(t, pipeline, 0, repeated(120, pumpMinute{2.5, 2, 30}), repeated(10, pumpMinute{3, 2, 60}))
	situationID := scalarText(t, db, "SELECT situation_id FROM situations WHERE entity_id = ?", pumpEntity)
	install(t, newWatch(t, db), watchOfPreset(t, compiled, situationID))

	runPumpMinutes(t, pipeline, 130, repeated(5, pumpMinute{3, 2, 60}))
	if fires := scalarText(t, db, "SELECT COUNT(*) FROM watch_fires"); fires != "0" {
		t.Fatalf("%s fires while vibration_mean_15m stays under 4.0, want none", fires)
	}
	runPumpMinutes(t, pipeline, 135, repeated(15, pumpMinute{8, 2, 60}))
	if status := scalarText(t, db, "SELECT status || ' ' || remaining_fires FROM watch_conditions"); status != "disabled 0" {
		t.Fatalf("watch after vibration_mean_15m reached 4.0 = %q, want its three fires spent", status)
	}
}

func repeated(count int, minute pumpMinute) []pumpMinute {
	minutes := make([]pumpMinute, count)
	for i := range minutes {
		minutes[i] = minute
	}
	return minutes
}

func runPumpMinutes(t *testing.T, pipeline *runtime.Pipeline, startMinute int, phases ...[]pumpMinute) {
	t.Helper()
	var lines []byte
	minute := startMinute
	for _, phase := range phases {
		for _, reading := range phase {
			lines = append(lines, pumpMinuteEvents(t, minute, reading)...)
			minute++
		}
	}
	path := filepath.Join(t.TempDir(), fmt.Sprintf("pump-%d.jsonl", startMinute))
	if err := os.WriteFile(path, lines, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := pipeline.RunJSONL(t.Context(), path); err != nil {
		t.Fatalf("run the pump trace from minute %d: %v", startMinute, err)
	}
}

func pumpMinuteEvents(t *testing.T, minute int, reading pumpMinute) []byte {
	t.Helper()
	at := pumpWorldStart.Add(time.Duration(minute) * time.Minute)
	var lines []byte
	for _, event := range []struct {
		kind, field string
		value       float64
	}{{"vibration", "rms_mm_s", reading.vibration}, {"axial_vibration", "value", reading.axial}, {"motor_current", "value", reading.current}} {
		line, err := json.Marshal(map[string]any{"id": fmt.Sprintf("pump-%s-%d", event.kind, minute), "type": "pump." + event.kind + ".observed",
			"schema_version": "1.1", "tenant_id": "default", "source": "plantwire", "partition_key": pumpEntity,
			"entity": map[string]any{"type": "pump", "id": pumpEntity}, "event_time": kernel.FormatTime(at), "ingested_at": kernel.FormatTime(at),
			"classification": "internal", "data": map[string]any{event.field: event.value, "quality": "valid", "boot_id": "boot-1", "seq": minute}})
		if err != nil {
			t.Fatal(err)
		}
		lines = append(append(lines, line...), '\n')
	}
	return lines
}

func watchOfPreset(t *testing.T, compiled *spec.CompiledSpec, situationID string) actionport.Command {
	t.Helper()
	for _, intent := range compiled.Actions.Intents {
		if intent.Type != "install_watch_condition" {
			continue
		}
		payload := map[string]any{"target": pumpEntity, "expires_at": "2099-01-01T00:00:00Z", "situation_id": situationID, "situation_version": 1}
		for key, value := range intent.Presets["default"] {
			payload[key] = value
		}
		return actionport.Command{CommandID: "cmd-pump-watch", TenantID: "default", EffectorRoute: "install_watch_condition", Payload: payload}
	}
	t.Fatal("the pump station spec has no install_watch_condition intent")
	return actionport.Command{}
}

func install(t *testing.T, service interface {
	Dispatch(context.Context, actionport.Command) (actionport.Effect, error)
}, command actionport.Command) {
	t.Helper()
	if _, err := service.Dispatch(t.Context(), command); err != nil {
		t.Fatalf("install %s: %v", command.CommandID, err)
	}
}

func scalarText(t *testing.T, db *storage.DB, query string, args ...any) string {
	t.Helper()
	var value string
	if err := db.QueryRowContext(t.Context(), query, args...).Scan(&value); err != nil {
		t.Fatalf("%s: %v", query, err)
	}
	return value
}
