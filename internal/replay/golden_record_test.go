package replay_test

import (
	"database/sql"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/replay"
)

var updateGolden = flag.Bool("update", false, "rewrite testdata/golden from the current replay results")

const emptyInputDigest = "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"

type goldenRecord struct {
	EventsProcessed int                 `json:"events_processed"`
	VersionCount    int                 `json:"version_count"`
	VersionsHash    string              `json:"versions_hash"`
	Phases          map[string][]string `json:"phases"`
	ExpectEmpty     bool                `json:"expect_empty,omitempty"`
}

type goldenTrace struct{ example, spec, trace string }

func goldenTraces() []goldenTrace {
	var traces []goldenTrace
	for _, name := range []string{"trace-opening.jsonl", "trace-watch.jsonl", "trace-heartbeat.jsonl"} {
		traces = append(traces, goldenTrace{"predictive-maintenance", predictiveSpec, "../../examples/predictive-maintenance/testdata/" + name})
	}
	for _, name := range []string{"trace-opening.jsonl", "trace-ambient-tracking.jsonl", "trace-invalid-quality.jsonl", "trace-quiet.jsonl", "trace-reboot-backlog.jsonl"} {
		traces = append(traces, goldenTrace{"thermal-chamber", thermalSpec, thermalTrace(name)})
	}
	return traces
}

func TestGoldenTracesMatchTheirRecordedResults(t *testing.T) {
	t.Parallel()
	for _, trace := range goldenTraces() {
		name := trace.example + "-" + strings.TrimSuffix(filepath.Base(trace.trace), ".jsonl")
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			got := replayRecord(t, trace)
			goldenPath := filepath.Join("testdata", "golden", name+".json")
			if *updateGolden {
				writeGolden(t, goldenPath, got)
				return
			}
			want := readGolden(t, goldenPath)
			got.ExpectEmpty = want.ExpectEmpty
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("replay of %s changed:\n got  %+v\n want %+v\n(rerun with -update only for a reviewed behavior change)", name, got, want)
			}
			if got.VersionsHash == emptyInputDigest && !want.ExpectEmpty {
				t.Fatalf("%s published no Situation versions; mark expect_empty in its golden file only if that is intended", name)
			}
		})
	}
}

func replayRecord(t *testing.T, trace goldenTrace) goldenRecord {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "replay.db")
	result, err := replay.Run(seededContext(t), replay.Request{DBPath: dbPath, SpecPath: trace.spec, TracePath: trace.trace, TenantID: "default"})
	if err != nil {
		t.Fatalf("replay %s: %v", trace.trace, err)
	}
	return goldenRecord{EventsProcessed: result.EventsProcessed, VersionCount: result.VersionCount, VersionsHash: result.VersionsHash, Phases: phaseSequences(t, dbPath)}
}

func phaseSequences(t *testing.T, dbPath string) map[string][]string {
	t.Helper()
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	rows, err := db.QueryContext(t.Context(), `
		SELECT s.entity_type || '/' || s.entity_id, v.phase FROM situation_versions v
		JOIN situations s ON s.situation_id = v.situation_id ORDER BY s.entity_type, s.entity_id, v.version`)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rows.Close() }()
	phases := make(map[string][]string)
	for rows.Next() {
		var entity, phase string
		if err := rows.Scan(&entity, &phase); err != nil {
			t.Fatal(err)
		}
		phases[entity] = append(phases[entity], phase)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return phases
}

func writeGolden(t *testing.T, path string, record goldenRecord) {
	t.Helper()
	record.ExpectEmpty = record.VersionsHash == emptyInputDigest
	encoded, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, append(encoded, '\n'), 0o600); err != nil {
		t.Fatal(err)
	}
}

func readGolden(t *testing.T, path string) goldenRecord {
	t.Helper()
	encoded, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("golden file %s: %v (generate it with -update)", path, err)
	}
	var record goldenRecord
	if err := json.Unmarshal(encoded, &record); err != nil {
		t.Fatal(fmt.Errorf("decode %s: %w", path, err))
	}
	if record.Phases == nil {
		record.Phases = map[string][]string{}
	}
	return record
}
