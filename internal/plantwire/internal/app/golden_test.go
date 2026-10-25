package app_test

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/contractsv1"
	"github.com/ghassan-ai-projects/agentic-stream/internal/plantwire/internal/app"
	"github.com/ghassan-ai-projects/agentic-stream/internal/testsupport/plantwirefake"
)

var benchExample = filepath.Join("..", "..", "..", "..", "examples", "real-world-sensor", "testdata", "plant-wire")

func TestTheBenchSiteFileShapesEventsLikeTheGoldenTrace(t *testing.T) {
	t.Parallel()
	kinds := filepath.Join(benchExample, "kinds")
	host := startHostWith(t, app.Config{Settings: simulatedHost, KindsDir: kinds, SitePath: filepath.Join(benchExample, "thermal-bench.site.json")}, nil)
	node := host.node(t, "s-1")
	node.Send(plantwirefake.Birth(t, kinds, "mega-01", "zone-01", "led-01", "fan-01"))
	node.Expect("STATE")
	node.Send(map[string]any{"type": "DBIRTH", "lane": "data", "device": "zone-01", "kind": "thermal_sensor", "board": "mega-01",
		"boot_id": "boot-A", "metadata_version": 1, "birth_time_ns": birthNS, "metrics": []any{
			map[string]any{"name": "temperature", "alias": 1, "value": 21, "quality": "good"},
			map[string]any{"name": "humidity", "alias": 2, "value": 40, "quality": "good"}}})
	node.Send(map[string]any{"type": "DDATA", "lane": "data", "device": "zone-01", "boot_id": "boot-A", "historical": false,
		"metrics": []any{map[string]any{"alias": 1, "value": 0, "quality": "disconnected", "event_time_ns": birthNS,
			"time_source": "device", "device_mono_us": 0}}})
	golden := goldenEvents(t)
	assertShapedLike(t, host.event(t), golden["iq-hb-000"], "boot_id", "quality")
	disconnected := host.event(t)
	assertShapedLike(t, disconnected, golden["iq-t-000"], "boot_id", "quality", "device_mono_us")
	if _, carried := disconnected.Data["celsius"]; carried || !slices.Equal(flagCodes(disconnected), flagCodes(golden["iq-t-000"])) {
		t.Errorf("disconnected reading = %+v, want no celsius and the golden quality flags", disconnected)
	}
}

func goldenEvents(t *testing.T) map[string]contractsv1.Envelope {
	t.Helper()
	file, err := os.Open(filepath.Join("..", "..", "..", "..", "examples", "thermal-chamber", "testdata", "trace-invalid-quality.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = file.Close() }()
	events := map[string]contractsv1.Envelope{}
	for lines := bufio.NewScanner(file); lines.Scan(); {
		var event contractsv1.Envelope
		if err := json.Unmarshal(lines.Bytes(), &event); err != nil {
			t.Fatal(err)
		}
		events[event.ID] = event
	}
	return events
}

func assertShapedLike(t *testing.T, event, golden contractsv1.Envelope, members ...string) {
	t.Helper()
	if event.Type != golden.Type || event.SchemaVersion != golden.SchemaVersion || event.Entity != golden.Entity ||
		event.PartitionKey != golden.PartitionKey || event.Classification != golden.Classification || !event.EventTime.Equal(golden.EventTime) {
		t.Fatalf("event = %+v, want the envelope of golden %s", event, golden.ID)
	}
	for _, member := range members {
		if canonical(t, event.Data[member]) != canonical(t, golden.Data[member]) {
			t.Errorf("%s data %s = %v, want %v", event.Type, member, event.Data[member], golden.Data[member])
		}
	}
	for member := range event.Data {
		if _, declared := golden.Data[member]; !declared && member != "schema_version" {
			t.Errorf("%s carries data %s, which golden %s does not", event.Type, member, golden.ID)
		}
	}
}

func canonical(t *testing.T, value any) string {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

func flagCodes(event contractsv1.Envelope) []string {
	var codes []string
	for _, flag := range event.Quality {
		codes = append(codes, flag.Code)
	}
	return codes
}
