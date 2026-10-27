package main

import (
	"context"
	"encoding/json"
	"errors"
	"maps"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/device"
	"github.com/ghassan-ai-projects/agentic-stream/internal/testsupport/plantwirefake"
)

const benchReadings = 6

var benchPlantWire = filepath.Join("..", "..", "examples", "real-world-sensor", "testdata", "plant-wire")

type plantWireServe struct {
	dir, socket, db string
	stop            func()
}

func startPlantWireServe(t *testing.T, specPath, kindsDir, sitePath string, extra ...string) plantWireServe {
	t.Helper()
	dir := privateSocketDir(t)
	run := plantWireServe{dir: dir, socket: filepath.Join(dir, "host.sock"), db: newMigratedDatabasePath(t)}
	address := freeLoopbackAddress(t)
	args := []string{"serve", "--db", run.db, "--spec", specPath, "--effect-profile", "emulator", "--plant-wire-socket", run.socket,
		"--plant-wire-kinds", kindsDir, "--plant-wire-epoch", "11", "--listen", address, "--poll-interval", "50ms"}
	if sitePath != "" {
		args = append(args, "--plant-wire-site", sitePath)
	}
	args = append(args, extra...)
	ctx, cancel := context.WithCancel(t.Context())
	cmd := newRootCommand()
	cmd.SetArgs(args)
	done := make(chan error, 1)
	go func() { done <- cmd.ExecuteContext(ctx) }()
	waitReady(t, "http://"+address+"/health/live", done)
	run.stop = func() {
		cancel()
		if err := <-done; err != nil {
			t.Errorf("serve returned %v after cancellation", err)
		}
	}
	t.Cleanup(run.stop)
	return run
}

func TestServeHostsPlantWireSessionsOneAfterAnother(t *testing.T) {
	t.Parallel()
	run := startPlantWireServe(t, testSpec, plantwirefake.KindsDir(t, "."), "")
	for _, session := range []string{"s-1", "s-2"} {
		node := plantwirefake.Dial(t, "unix", run.socket, session)
		node.Send(plantwirefake.Fixture(t, ".", "NBIRTH.json"))
		if state := node.Expect("STATE"); state["online"] != true || state["authority_epoch"] != json.Number("11") {
			t.Fatalf("%s: STATE = %v, want online at epoch 11", session, state)
		}
		node.Send(map[string]any{"type": "NSYNC", "lane": "control", "through_seq": 0, "world_time_ns": 1767225660000000000})
		node.Expect("HSYNC")
		node.Send(map[string]any{"type": "NDEATH", "lane": "control", "bd_seq": 0, "reason": "shutdown"})
		if !node.Closed() {
			t.Fatalf("%s: the host kept the session after the node's death", session)
		}
	}
}

func TestServeIngestsTheBenchReadingsOfAPlantWireNodeAndOpensASituation(t *testing.T) {
	t.Parallel()
	kinds := filepath.Join(benchPlantWire, "kinds")
	run := startPlantWireServe(t, benchSpec, kinds, filepath.Join(benchPlantWire, "thermal-bench.site.json"))
	node := plantwirefake.Dial(t, "unix", run.socket, "bench-1")
	node.Send(plantwirefake.Birth(t, kinds, "mega-01", "zone-01", "led-01", "fan-01"))
	node.Expect("STATE")
	start := time.Now().Add(-3 * time.Minute).Truncate(time.Second)
	node.Send(map[string]any{"type": "DBIRTH", "lane": "data", "device": "zone-01", "kind": "thermal_sensor", "board": "mega-01",
		"boot_id": "boot-A", "metadata_version": 1, "birth_time_ns": start.UnixNano(), "metrics": []any{
			map[string]any{"name": "temperature", "alias": 1, "value": nil, "quality": "warming_up"},
			map[string]any{"name": "humidity", "alias": 2, "value": nil, "quality": "warming_up"}}})
	for step := range benchReadings {
		at := start.Add(time.Duration(step) * 20 * time.Second).UnixNano()
		node.Send(map[string]any{"type": "DDATA", "lane": "data", "device": "zone-01", "boot_id": "boot-A", "historical": false,
			"metrics": []any{benchSample(1, 31+float64(step), at), benchSample(2, 40, at)}})
	}
	node.Send(map[string]any{"type": "NSYNC", "lane": "control", "through_seq": 0, "world_time_ns": start.UnixNano()})
	node.Expect("HSYNC")
	want := map[string]int{"zone.temp.observed": benchReadings, "zone.humidity.observed": benchReadings, "zone.heartbeat.observed": benchReadings}
	if !pollUntil(t, readyTimeout, func() bool { return maps.Equal(eventTypeCounts(t, run.db), want) }) {
		t.Fatalf("event log = %v, want %v", eventTypeCounts(t, run.db), want)
	}
	if situations := countRows(t, run.db, "situations"); situations == 0 {
		t.Fatal("the bench readings opened no situation")
	}
	if quarantined := countRows(t, run.db, "event_quarantine"); quarantined != 0 {
		t.Fatalf("%d events quarantined, want none", quarantined)
	}
}

func benchSample(alias int, value float64, atNS int64) map[string]any {
	return map[string]any{"alias": alias, "value": value, "quality": "good", "event_time_ns": atNS, "time_source": "device"}
}

func eventTypeCounts(t *testing.T, dbPath string) map[string]int {
	t.Helper()
	return countsBy(t, dbPath, "SELECT event_type, COUNT(*) FROM event_log GROUP BY event_type")
}

func countsBy(t *testing.T, dbPath, query string) map[string]int {
	t.Helper()
	rows, err := openReadOnly(t, dbPath).QueryContext(t.Context(), query)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rows.Close() }()
	counts := map[string]int{}
	for rows.Next() {
		var key string
		var count int
		if err := rows.Scan(&key, &count); err != nil {
			t.Fatal(err)
		}
		counts[key] = count
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return counts
}

func TestServeChecksThePlantWireRoutesAgainstTheDispatchLease(t *testing.T) {
	t.Parallel()
	kinds := plantwirefake.KindsDir(t, ".")
	dir := privateSocketDir(t)
	cmd := newServeCommand()
	cmd.SetArgs([]string{"--db", newMigratedDatabasePath(t), "--spec", testSpec, "--effect-profile", "emulator",
		"--plant-wire-socket", filepath.Join(dir, "host.sock"), "--plant-wire-kinds", kinds, "--device-catalog", experimentCatalog,
		"--listen", freeLoopbackAddress(t)})
	if err := cmd.ExecuteContext(t.Context()); !errors.Is(err, device.ErrRouteOutlivesDispatch) || !strings.Contains(err.Error(), "set_indicator") {
		t.Fatalf("serve under the default lease = %v, want the 60 s set_indicator route refused", err)
	}
	run := startPlantWireServe(t, testSpec, kinds, experimentSite(t), "--device-catalog", experimentCatalog, "--dispatch-lease", "2m")
	node := plantwirefake.Dial(t, "unix", run.socket, "s-1")
	node.Send(plantwirefake.Birth(t, kinds, "board-01", "led-01", "fan-01"))
	node.Expect("STATE")
}

func experimentSite(t *testing.T) string {
	t.Helper()
	site := `{"version":1,"source":"plantwire-test","devices":{"led-01":{"kind":"pump"},"fan-01":{"kind":"pump"}},` +
		`"readings":[],"heartbeats":[],"quality":{"good":"valid","warming_up":"warming","suspect":"invalid",` +
		`"out_of_range":"invalid","stale":"invalid","disconnected":"disconnected","manual_override":"invalid",` +
		`"out_of_service":"disconnected"}}`
	path := filepath.Join(t.TempDir(), "experiment.site.json")
	writePrivateFile(t, path, site)
	return path
}

func TestServeMakesThePlantWireHostTheDeviceEffectorOfItsCatalog(t *testing.T) {
	t.Parallel()
	kinds, site := filepath.Join(benchPlantWire, "kinds"), filepath.Join(benchPlantWire, "thermal-bench.site.json")
	raw, err := os.ReadFile(filepath.Join(benchPlantWire, "thermal-bench.catalog.json"))
	if err != nil {
		t.Fatal(err)
	}
	partial := strings.Replace(string(raw), `"select_thermal_mode"`, `"open_the_window"`, 1)
	catalog := filepath.Join(t.TempDir(), "partial.catalog.json")
	writePrivateFile(t, catalog, partial)
	cmd := newServeCommand()
	cmd.SetArgs([]string{"--db", newMigratedDatabasePath(t), "--spec", benchSpec, "--effect-profile", "emulator",
		"--plant-wire-socket", filepath.Join(privateSocketDir(t), "host.sock"), "--plant-wire-kinds", kinds, "--plant-wire-site", site,
		"--device-catalog", catalog, "--listen", freeLoopbackAddress(t)})
	if err := cmd.ExecuteContext(t.Context()); err == nil || !strings.Contains(err.Error(), `route "select_thermal_mode" has no effector`) {
		t.Fatalf("serve with a catalog missing a dispatched route = %v, want the route refused at start", err)
	}
	startPlantWireServe(t, benchSpec, kinds, site, "--device-catalog", filepath.Join(benchPlantWire, "thermal-bench.catalog.json"))
}

func writePrivateFile(t *testing.T, path, content string) {
	t.Helper()
	root, err := os.OpenRoot(filepath.Dir(path))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = root.Close() }()
	if err := root.WriteFile(filepath.Base(path), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}
