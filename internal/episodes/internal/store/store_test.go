package store_test

import (
	"bufio"
	"context"
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/decisions"
	"github.com/ghassan-ai-projects/agentic-stream/internal/engine"
	"github.com/ghassan-ai-projects/agentic-stream/internal/episodeledger"
	"github.com/ghassan-ai-projects/agentic-stream/internal/episodes"
	"github.com/ghassan-ai-projects/agentic-stream/internal/episodes/internal/store"
	"github.com/ghassan-ai-projects/agentic-stream/internal/eventlog"
	"github.com/ghassan-ai-projects/agentic-stream/internal/ingress"
	"github.com/ghassan-ai-projects/agentic-stream/internal/sources"
	"github.com/ghassan-ai-projects/agentic-stream/internal/spec"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
)

const (
	fixtureSpecPath  = "../../../../docs/design/examples/predictive-maintenance.situation.yaml"
	fixtureTracePath = "../../../../examples/predictive-maintenance/testdata/trace-opening.jsonl"
)

// replayedStore drives one worker-aware replay through the lower modules so
// store tests read and write real episode rows without importing the facade's
// use cases into the store package graph.
func replayedStore(t *testing.T) *storage.DB {
	t.Helper()
	ctx := context.Background()
	db, err := storage.Open(ctx, filepath.Join(t.TempDir(), "episodes.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
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
	return db
}

func admitOne(t *testing.T, db *storage.DB, assembler *episodes.Service) {
	t.Helper()
	ctx := context.Background()
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

func TestDispatchableEpisodeReadsOldestAdmittedEpisode(t *testing.T) {
	t.Parallel()
	db := replayedStore(t)
	ctx := context.Background()
	var episode store.DispatchedEpisode
	if err := db.WithTx(ctx, func(tx *sql.Tx) error {
		var err error
		episode, err = store.DispatchableEpisode(ctx, store.Join(tx), "default", false)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if episode.EpisodeID == "" || episode.TenantID != "default" || len(episode.RequestJSON) == 0 || episode.StaleRebindCount != 0 {
		t.Fatalf("dispatched episode = %+v", episode)
	}
	if err := db.WithTx(ctx, func(tx *sql.Tx) error {
		_, err := store.DispatchableEpisode(ctx, store.Join(tx), "missing-tenant", false)
		return err
	}); err == nil {
		t.Fatal("unknown tenant produced an episode")
	}
}

func TestLifecycleReadsAndAttemptCounts(t *testing.T) {
	t.Parallel()
	db := replayedStore(t)
	ctx := context.Background()
	var episodeID string
	if err := db.QueryRowContext(ctx, "SELECT episode_id FROM episodes LIMIT 1").Scan(&episodeID); err != nil {
		t.Fatal(err)
	}
	var lifecycle string
	if err := db.WithTx(ctx, func(tx *sql.Tx) error {
		var err error
		lifecycle, err = store.EpisodeLifecycle(ctx, store.Join(tx), episodeID)
		return err
	}); err != nil || lifecycle != "admitted" {
		t.Fatalf("lifecycle = %q err = %v", lifecycle, err)
	}
	if store.New(db).EpisodeSupersededNow(ctx, episodeID) {
		t.Fatal("admitted episode reported superseded")
	}
	var failed int
	if err := db.WithTx(ctx, func(tx *sql.Tx) error {
		var err error
		failed, err = store.CountFailedAttempts(ctx, store.Join(tx), episodeID)
		return err
	}); err != nil || failed != 0 {
		t.Fatalf("failed attempts = %d err = %v", failed, err)
	}
	if err := db.WithTx(ctx, func(tx *sql.Tx) error {
		_, err := store.AttemptStatus(ctx, store.Join(tx), episodeledger.Identity{EpisodeID: episodeID, AttemptID: "att-missing", Fence: 1})
		return err
	}); err == nil {
		t.Fatal("missing attempt status read succeeded")
	}
}

func TestSchedulerEvaluationAndSnapshotLoads(t *testing.T) {
	t.Parallel()
	db := replayedStore(t)
	ctx := context.Background()
	var itemID string
	if err := db.QueryRowContext(ctx, "SELECT scheduler_item_id FROM scheduler_items LIMIT 1").Scan(&itemID); err != nil {
		t.Fatal(err)
	}
	if err := db.WithTx(ctx, func(tx *sql.Tx) error {
		item, err := store.LoadSchedulerItem(ctx, store.Join(tx), itemID)
		if err != nil {
			return err
		}
		if item.TenantID != "default" || item.SituationID == "" {
			t.Fatalf("scheduler item = %+v", item)
		}
		if _, err := store.LoadEvaluation(ctx, store.Join(tx), item.TriggerID); err != nil {
			return err
		}
		_, _, _, _, err = store.LoadSnapshot(ctx, store.Join(tx), item.SituationID, item.SituationVersion)
		if err != nil {
			return err
		}
		live, err := store.LiveSituationVersion(ctx, store.Join(tx), item.TenantID, item.SituationID)
		if err != nil || live < 1 {
			t.Fatalf("live version = %d err = %v", live, err)
		}
		_, err = store.LoadSchedulerItem(ctx, store.Join(tx), "missing-item")
		return err
	}); err == nil {
		t.Fatal("missing scheduler item accepted")
	}
}

func TestDecisionAndIntentInsertsAnnotateAndAccept(t *testing.T) {
	t.Parallel()
	db := replayedStore(t)
	ctx := context.Background()
	var episodeID string
	if err := db.QueryRowContext(ctx, "SELECT episode_id FROM episodes LIMIT 1").Scan(&episodeID); err != nil {
		t.Fatal(err)
	}
	var situationID string
	var situationVersion int
	if err := db.QueryRowContext(ctx, "SELECT situation_id, situation_version FROM episodes WHERE episode_id = ?", episodeID).Scan(&situationID, &situationVersion); err != nil {
		t.Fatal(err)
	}
	err := db.WithTx(ctx, func(tx *sql.Tx) error {
		identity, err := episodeledger.StartAttempt(ctx, tx, episodeID, "att-1", time.Unix(0, 0).UTC())
		if err != nil {
			return err
		}
		if err := store.InsertDecision(ctx, store.Join(tx), store.DecisionInsert{
			DecisionID: "dec-1", EpisodeID: episodeID, AttemptID: identity.AttemptID, Fence: identity.Fence,
			SituationID: situationID, SituationVersion: situationVersion, RawJSON: []byte(`{"decision_id":"dec-1"}`),
			Digest: bytesOf(1), ValidationStatus: "proposed", ValidationJSON: []byte(`{}`), Now: "2026-01-01T00:00:00Z",
		}); err != nil {
			return err
		}
		if err := store.AnnotateRejectedDecision(ctx, store.Join(tx), "dec-1", "schema_invalid"); err != nil {
			return err
		}
		return store.AcceptDecision(ctx, store.Join(tx), "dec-1")
	})
	if err != nil {
		t.Fatal(err)
	}
	var status, reason string
	if err := db.QueryRowContext(ctx, "SELECT validation_status, rejection_reason FROM decisions WHERE decision_id = 'dec-1'").Scan(&status, &reason); err != nil {
		t.Fatal(err)
	}
	if status != "accepted" || reason != "schema_invalid" {
		t.Fatalf("decision = (%q, %q)", status, reason)
	}
	err = db.WithTx(ctx, func(tx *sql.Tx) error {
		return store.InsertValidatedIntent(ctx, store.Join(tx), store.ValidatedIntentInsert{
			Intent:     intentFixture(),
			DecisionID: "dec-1", TenantID: "default", SituationID: situationID, SituationVersion: situationVersion,
			Now: "2026-01-01T00:00:00Z",
		})
	})
	if err != nil {
		t.Fatal(err)
	}
	var policyStatus string
	if err := db.QueryRowContext(ctx, "SELECT policy_status FROM intents WHERE intent_id = 'int-1'").Scan(&policyStatus); err != nil {
		t.Fatal(err)
	}
	if policyStatus != "pending" {
		t.Fatalf("intent policy status = %q", policyStatus)
	}
}

// alwaysTriggerSpec rewrites the predictive fixture so cognition triggers on
// every update, guaranteeing a scheduler item to assemble from.
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
