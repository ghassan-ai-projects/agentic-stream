package store_test

import (
	"bufio"
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/decisions"
	"github.com/ghassan-ai-projects/agentic-stream/internal/engine"
	"github.com/ghassan-ai-projects/agentic-stream/internal/episodes"
	"github.com/ghassan-ai-projects/agentic-stream/internal/eventlog"
	"github.com/ghassan-ai-projects/agentic-stream/internal/ingress"
	"github.com/ghassan-ai-projects/agentic-stream/internal/sources"
	"github.com/ghassan-ai-projects/agentic-stream/internal/spec"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage/storagetest"
)

const (
	fixtureSpecPath  = "../../../../docs/design/examples/predictive-maintenance.situation.yaml"
	fixtureTracePath = "../../../../examples/predictive-maintenance/testdata/trace-opening.jsonl"
)

var replayed struct {
	once     sync.Once
	database []byte
}

func replayedStore(t *testing.T) *storage.DB {
	t.Helper()
	replayed.once.Do(func() { replayed.database = buildReplayedDatabase(t) })
	if len(replayed.database) == 0 {
		t.Fatal("the replayed episode database could not be built")
	}
	path := filepath.Join(t.TempDir(), "runtime.db")
	if err := os.WriteFile(path, replayed.database, 0o600); err != nil {
		t.Fatal(err)
	}
	db, err := storagetest.Open(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func buildReplayedDatabase(t *testing.T) []byte {
	t.Helper()
	ctx := t.Context()
	path := filepath.Join(t.TempDir(), "template.db")
	db, err := storagetest.Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}

	compiled, err := spec.CompileFile(ctx, alwaysTriggerSpec(t))
	if err != nil {
		t.Fatal(err)
	}
	if err := spec.SaveDeployment(ctx, db, "default", compiled); err != nil {
		t.Fatal(err)
	}
	clk := sources.NewVirtual(firstTraceTime(t))
	log := eventlog.NewEventLogWithClock(db, clk)
	log.RequireSchemaValidation()
	ingestor, err := ingress.New(ingress.Config{DB: db, Log: log, Clock: clk, TenantID: "default"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ingestor.ReplayJSONL(ctx, fixtureTracePath, "replay:"+fixtureTracePath); err != nil {
		t.Fatal(err)
	}
	eng, err := engine.New(ctx, engine.Config{DB: db, Log: log, Clock: clk, Spec: compiled, TenantID: "default", RuntimeOwner: engine.ReplayOwnership, Cognition: true})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := eng.RunGlobal(ctx, func(record eventlog.Record) error {
		processingTime := record.IngestedAt.UTC()
		if processingTime.IsZero() {
			processingTime = record.EventTime.UTC()
		}
		if processingTime.After(clk.Now()) {
			clk.Advance(processingTime.Sub(clk.Now()))
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	assembler, err := episodes.New(episodes.Config{Spec: compiled, IDGenerator: sources.Deterministic()})
	if err != nil {
		t.Fatal(err)
	}
	admitOne(t, db, assembler)
	if _, err := db.ExecContext(ctx, "PRAGMA wal_checkpoint(TRUNCATE)"); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	built, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return built
}

func admitOne(t *testing.T, db *storage.DB, assembler *episodes.Service) {
	t.Helper()
	ctx := t.Context()
	var itemID string
	if err := db.QueryRowContext(ctx, "SELECT scheduler_item_id FROM scheduler_items WHERE status = 'pending' ORDER BY scheduler_item_id LIMIT 1").Scan(&itemID); err != nil {
		t.Fatalf("find pending scheduler item: %v", err)
	}
	if err := db.WithTx(ctx, func(tx *sql.Tx) error {
		req, err := assembler.Assemble(ctx, tx, itemID, "default")
		if err != nil {
			return err
		}
		return assembler.Persist(ctx, tx, req, time.Unix(0, 0).UTC())
	}); err != nil {
		t.Fatalf("admit episode: %v", err)
	}
}

func alwaysTriggerSpec(t *testing.T) string {
	t.Helper()
	specBytes, err := os.ReadFile(fixtureSpecPath)
	if err != nil {
		t.Fatal(err)
	}
	specText := strings.Replace(string(specBytes),
		"when: >\n        situation.phase in [\"warning\", \"incident\"] &&\n        (!has(features.heartbeat_missing_5m) || features.heartbeat_missing_5m == false)", "when: true", 1)
	specText = strings.Replace(specText,
		"score: >\n        double(situation.severity) * 0.5 +\n        double(delta.novelty) * 25.0 +\n        double(situation.uncertainty) * 25.0", "score: 100.0", 1)
	specText = strings.Replace(specText,
		"materialDelta: >\n        delta.phase_changed ||\n        delta.severity_change >= 10 ||\n        delta.primary_hypothesis_changed ||\n        delta.completeness_changed", "materialDelta: true", 1)
	specText = strings.Replace(specText, "      debounce: 2m\n", "", 1)
	specText = strings.Replace(specText, "      cooldown: 30m\n", "", 1)
	specText = strings.Replace(specText, "emit: on_close", "emit: on_update", 1)
	path := filepath.Join(t.TempDir(), "always-trigger.situation.yaml")
	if err := os.WriteFile(path, []byte(specText), 0o600); err != nil { //nolint:gosec // test path is created under t.TempDir().
		t.Fatal(err)
	}
	return path
}

func firstTraceTime(t *testing.T) time.Time {
	t.Helper()
	file, err := os.Open(fixtureTracePath)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = file.Close() }()
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		if len(scanner.Bytes()) > 0 {
			var envelope struct {
				IngestedAt time.Time `json:"ingested_at"`
				EventTime  time.Time `json:"event_time"`
			}
			if err := json.Unmarshal(scanner.Bytes(), &envelope); err != nil {
				continue
			}
			if !envelope.IngestedAt.IsZero() {
				return envelope.IngestedAt.UTC()
			}
			if !envelope.EventTime.IsZero() {
				return envelope.EventTime.UTC()
			}
		}
	}
	t.Fatal("trace has no valid time")
	return time.Time{}
}

func intentFixture() decisions.Intent {
	return decisions.Intent{
		ID: "int-1", Type: "create_maintenance_ticket", RiskClass: "R1",
		CanonicalJSON: []byte(`{}`), Digest: "sha256:" + strings.Repeat("3", 64),
		ExpiresAt: time.Date(2099, 1, 1, 0, 0, 0, 0, time.UTC),
	}
}

func bytesOf(fill byte) []byte {
	value := make([]byte, 32)
	for i := range value {
		value[i] = fill
	}
	return value
}

func admittedEpisodeID(t *testing.T, db *storage.DB) string {
	t.Helper()
	var episodeID string
	if err := db.QueryRowContext(t.Context(), "SELECT episode_id FROM episodes").Scan(&episodeID); err != nil {
		t.Fatalf("the replay must admit exactly one episode: %v", err)
	}
	return episodeID
}

func scalar[T any](t *testing.T, db *storage.DB, query string, args ...any) T {
	t.Helper()
	var value T
	if err := db.QueryRowContext(t.Context(), query, args...).Scan(&value); err != nil {
		t.Fatalf("query %q: %v", query, err)
	}
	return value
}
