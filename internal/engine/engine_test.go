package engine_test

import (
	"context"
	"database/sql"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/clock"
	"github.com/ghassan-ai-projects/agentic-stream/internal/contractsv1"
	"github.com/ghassan-ai-projects/agentic-stream/internal/engine"
	"github.com/ghassan-ai-projects/agentic-stream/internal/eventlog"
	"github.com/ghassan-ai-projects/agentic-stream/internal/spec"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
	_ "modernc.org/sqlite"
)

func TestEngineAdvancesCheckpoint(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	db, err := storage.Open(ctx, filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer func() { _ = db.Close() }()

	compiled, err := spec.CompileFile(ctx, "../../docs/design/examples/predictive-maintenance.situation.yaml")
	if err != nil {
		t.Fatalf("compile spec: %v", err)
	}

	log := eventlog.NewEventLog(db)
	clk := clock.Physical()
	eng, err := engine.NewEngine(ctx, db, log, clk, compiled, "default")
	if err != nil {
		t.Fatalf("new engine: %v", err)
	}

	env := contractsv1.Envelope{
		ID:             "evt-1",
		Type:           "motor.vibration.observed",
		SchemaVersion:  "1.0",
		TenantID:       "default",
		Source:         "simulator",
		PartitionKey:   "motor-17",
		Entity:         contractsv1.EntityRef{Type: "motor", ID: "motor-17"},
		EventTime:      time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		IngestedAt:     time.Date(2026, 1, 1, 0, 0, 1, 0, time.UTC),
		Classification: contractsv1.ClassificationInternal,
		Data:           map[string]any{"rms_mm_s": 5.0},
	}
	if _, err := log.Append(ctx, "default", []contractsv1.Envelope{env}); err != nil {
		t.Fatalf("append event: %v", err)
	}

	partitionID := env.PartitionID(0)
	processed, err := eng.Run(ctx, partitionID)
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}
	if processed != 1 {
		t.Fatalf("expected 1 processed event, got %d", processed)
	}

	// A second run should process nothing because the inbox record exists.
	processed, err = eng.Run(ctx, partitionID)
	if err != nil {
		t.Fatalf("second Run failed: %v", err)
	}
	if processed != 0 {
		t.Fatalf("expected 0 processed events on replay, got %d", processed)
	}
}

func TestEngineRetriesApplyAfterTransientSQLiteBusy(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "apply-contention.db")
	db, err := storage.Open(ctx, dbPath)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer func() { _ = db.Close() }()

	// Make the database connection fail fast so this test proves the engine's
	// application retry rather than waiting for SQLite's production timeout.
	db.SetMaxOpenConns(1)
	if _, err := db.ExecContext(ctx, "PRAGMA busy_timeout = 1"); err != nil {
		t.Fatalf("set test busy timeout: %v", err)
	}

	compiled := restartSpec()
	log := eventlog.NewEventLog(db)
	eng, err := engine.NewStreamEngine(ctx, db, log, clock.Physical(), &compiled, "default")
	if err != nil {
		t.Fatalf("new engine: %v", err)
	}
	appendLevel(t, ctx, log, "evt-contention", 0, 15)

	lockerDB, err := sql.Open("sqlite", dbPath+"?_pragma=busy_timeout(1)")
	if err != nil {
		t.Fatalf("open lock connection: %v", err)
	}
	defer func() { _ = lockerDB.Close() }()
	locker, err := lockerDB.Conn(ctx)
	if err != nil {
		t.Fatalf("acquire lock connection: %v", err)
	}
	defer func() { _ = locker.Close() }()
	if _, err := locker.ExecContext(ctx, "BEGIN IMMEDIATE"); err != nil {
		t.Fatalf("begin competing writer: %v", err)
	}
	if _, err := locker.ExecContext(ctx, "UPDATE event_log SET source = source WHERE event_id = ?", "evt-contention"); err != nil {
		t.Fatalf("hold competing writer: %v", err)
	}

	type result struct {
		processed int
		err       error
	}
	done := make(chan result, 1)
	go func() {
		processed, runErr := eng.RunGlobal(ctx, nil)
		done <- result{processed: processed, err: runErr}
	}()

	timer := time.NewTimer(100 * time.Millisecond)
	<-timer.C
	if _, err := locker.ExecContext(context.Background(), "ROLLBACK"); err != nil {
		t.Fatalf("release competing writer: %v", err)
	}

	select {
	case result := <-done:
		if result.err != nil {
			t.Fatalf("engine failed after transient SQLite busy: %v", result.err)
		}
		if result.processed != 1 {
			t.Fatalf("processed=%d, want 1", result.processed)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("engine did not finish after competing writer released the database")
	}
}

func TestEngineRestoresSituationStateAcrossRestart(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "restart.db")
	compiled := restartSpec()
	clk := clock.NewVirtual(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))

	db, err := storage.Open(ctx, dbPath)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	log := eventlog.NewEventLogWithClock(db, clk)
	eng, err := engine.NewStreamEngine(ctx, db, log, clk, &compiled, "default")
	if err != nil {
		t.Fatalf("new engine: %v", err)
	}
	partitionID := appendLevel(t, ctx, log, "evt-1", 0, 15)
	if processed, err := eng.Run(ctx, partitionID); err != nil || processed != 1 {
		t.Fatalf("first run: processed=%d err=%v", processed, err)
	}
	var firstID string
	if err := db.QueryRowContext(ctx, "SELECT situation_id FROM situations").Scan(&firstID); err != nil {
		t.Fatalf("read first situation: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close first db: %v", err)
	}
	legacyDB, err := storage.Open(ctx, dbPath)
	if err != nil {
		t.Fatalf("reopen for incompatibility check: %v", err)
	}
	if _, err := legacyDB.ExecContext(ctx, "UPDATE situations SET state_codec_version = 0"); err != nil {
		t.Fatalf("mark unsupported state codec: %v", err)
	}
	legacyLog := eventlog.NewEventLogWithClock(legacyDB, clk)
	if _, err := engine.NewStreamEngine(ctx, legacyDB, legacyLog, clk, &compiled, "default"); err == nil || !strings.Contains(err.Error(), "requires rebuild") {
		t.Fatalf("expected legacy state to be refused, got %v", err)
	}
	if _, err := legacyDB.ExecContext(ctx, "UPDATE situations SET state_codec_version = 1"); err != nil {
		t.Fatalf("restore supported state codec for continuation test: %v", err)
	}
	if err := legacyDB.Close(); err != nil {
		t.Fatalf("close incompatibility-check db: %v", err)
	}

	db, err = storage.Open(ctx, dbPath)
	if err != nil {
		t.Fatalf("reopen db: %v", err)
	}
	defer func() { _ = db.Close() }()
	log = eventlog.NewEventLogWithClock(db, clk)
	eng, err = engine.NewStreamEngine(ctx, db, log, clk, &compiled, "default")
	if err != nil {
		t.Fatalf("restore engine: %v", err)
	}
	partitionID = appendLevel(t, ctx, log, "evt-2", time.Minute, 16)
	if processed, err := eng.Run(ctx, partitionID); err != nil || processed != 1 {
		t.Fatalf("second run: processed=%d err=%v", processed, err)
	}
	var secondID string
	var version int
	if err := db.QueryRowContext(ctx, "SELECT situation_id, current_version FROM situations").Scan(&secondID, &version); err != nil {
		t.Fatalf("read restored situation: %v", err)
	}
	if secondID != firstID {
		t.Fatalf("situation identity changed across restart: first=%s second=%s", firstID, secondID)
	}
	if version != 2 {
		t.Fatalf("expected restored situation to advance to version 2, got %d", version)
	}
	appendLevel(t, ctx, log, "evt-3", 2*time.Minute, 20)
	if processed, err := eng.Run(ctx, partitionID); err != nil || processed != 1 {
		t.Fatalf("non-version state run: processed=%d err=%v", processed, err)
	}
	var stateJSON string
	if err := db.QueryRowContext(ctx, "SELECT current_version, state_json FROM situations").Scan(&version, &stateJSON); err != nil {
		t.Fatalf("read non-version state: %v", err)
	}
	if version != 2 || !strings.Contains(stateJSON, `"facts.level":20`) {
		t.Fatalf("expected reducer state to persist without version, version=%d state=%s", version, stateJSON)
	}
}

func TestEngineFiresDurableProcessingTimerExactlyOnce(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	clk := clock.NewVirtual(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	compiled := heartbeatSpec()
	db, err := storage.Open(ctx, filepath.Join(dir, "timer.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer func() { _ = db.Close() }()
	log := eventlog.NewEventLogWithClock(db, clk)
	eng, err := engine.NewStreamEngine(ctx, db, log, clk, &compiled, "default")
	if err != nil {
		t.Fatalf("new engine: %v", err)
	}
	partitionID := appendHeartbeat(t, ctx, log, "hb-1", 0)
	if processed, err := eng.Run(ctx, partitionID); err != nil || processed != 1 {
		t.Fatalf("heartbeat run: processed=%d err=%v", processed, err)
	}
	var status string
	if err := db.QueryRowContext(ctx, "SELECT status FROM timers").Scan(&status); err != nil {
		t.Fatalf("read scheduled timer: %v", err)
	}
	if status != "pending" {
		t.Fatalf("expected pending timer, got %s", status)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close before timer restart: %v", err)
	}
	db, err = storage.Open(ctx, filepath.Join(dir, "timer.db"))
	if err != nil {
		t.Fatalf("reopen timer db: %v", err)
	}
	defer func() { _ = db.Close() }()
	log = eventlog.NewEventLogWithClock(db, clk)
	eng, err = engine.NewStreamEngine(ctx, db, log, clk, &compiled, "default")
	if err != nil {
		t.Fatalf("restore timer engine: %v", err)
	}
	clk.Advance(5 * time.Minute)
	fired, err := eng.RunDueTimers(ctx, partitionID)
	if err != nil {
		t.Fatalf("fire timer: %v", err)
	}
	if fired != 1 {
		t.Fatalf("expected one fired timer, got %d", fired)
	}
	fired, err = eng.RunDueTimers(ctx, partitionID)
	if err != nil {
		t.Fatalf("refire timer: %v", err)
	}
	if fired != 0 {
		t.Fatalf("expected fired timer to be idempotent, got %d", fired)
	}
	appendHeartbeat(t, ctx, log, "hb-2", time.Minute)
	if processed, err := eng.Run(ctx, partitionID); err != nil || processed != 1 {
		t.Fatalf("post-fire heartbeat run: processed=%d err=%v", processed, err)
	}
	var firedTimers, pendingTimers int
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM timers WHERE status = 'fired'").Scan(&firedTimers); err != nil {
		t.Fatalf("count fired timers: %v", err)
	}
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM timers WHERE status = 'pending'").Scan(&pendingTimers); err != nil {
		t.Fatalf("count pending timers: %v", err)
	}
	if firedTimers != 1 || pendingTimers != 1 {
		t.Fatalf("expected fired timer to remain fired and one new pending timer, fired=%d pending=%d", firedTimers, pendingTimers)
	}
	var versions int
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM situation_versions").Scan(&versions); err != nil {
		t.Fatalf("count timer versions: %v", err)
	}
	if versions != 2 {
		t.Fatalf("expected timer-derived uncertainty and recovery versions, got %d", versions)
	}
	var completeness string
	if err := db.QueryRowContext(ctx, "SELECT completeness FROM situation_versions WHERE version = 1").Scan(&completeness); err != nil {
		t.Fatalf("read source-health completeness: %v", err)
	}
	if completeness != "uncertain" {
		t.Fatalf("expected missing heartbeat to mark dependent situation uncertain, got %q", completeness)
	}
	if err := db.QueryRowContext(ctx, "SELECT completeness FROM situation_versions WHERE version = 2").Scan(&completeness); err != nil {
		t.Fatalf("read source-health recovery completeness: %v", err)
	}
	if completeness != "on_time" {
		t.Fatalf("expected heartbeat recovery to restore on_time completeness, got %q", completeness)
	}
	var stateJSON string
	if err := db.QueryRowContext(ctx, "SELECT state_json FROM situations").Scan(&stateJSON); err != nil {
		t.Fatalf("read timer situation state: %v", err)
	}
	if !strings.Contains(stateJSON, `"facts.missing":true`) {
		t.Fatalf("expected latest_event_time reducer to retain missing=true, state=%s", stateJSON)
	}
}

func restartSpec() spec.CompiledSpec {
	return spec.CompiledSpec{
		SchemaVersion: "agentic-stream/v1", Digest: "sha256:0000000000000000000000000000000000000000000000000000000000000000",
		Time:      spec.TimePolicy{MaxOutOfOrderness: "0s"},
		Inputs:    []spec.Input{{Name: "level", EventType: "test.level", EntityType: "thing"}},
		Windows:   []spec.Window{{Name: "tiny", Kind: "tumbling", Size: "1m", Emit: "early_and_close"}},
		Operators: []spec.Operator{{Name: "level_max", Kind: "aggregate", Inputs: []string{"level"}, Field: "data.level", Aggregate: "max", Window: "tiny", Output: "level"}},
		Situation: spec.Situation{Type: "test", InitialPhase: "candidate", Phases: []spec.Phase{{Name: "candidate", Severity: 10}, {Name: "watch", Severity: 30}, {Name: "warning", Severity: 60}}, Transitions: []spec.Transition{{From: "candidate", To: "watch", When: "features.level > 10", MinDuration: "0s"}, {From: "watch", To: "warning", When: "features.level > 10", MinDuration: "0s"}}, Occurrence: spec.Occurrence{OpenWhen: "features.level > 10"}, Reducers: []spec.Reducer{{Field: "facts.level", Strategy: "latest_event_time", Input: "level"}}},
	}
}

func heartbeatSpec() spec.CompiledSpec {
	return spec.CompiledSpec{
		SchemaVersion: "agentic-stream/v1", Digest: "sha256:0000000000000000000000000000000000000000000000000000000000000000",
		Time:      spec.TimePolicy{MaxOutOfOrderness: "0s"},
		Inputs:    []spec.Input{{Name: "heartbeat", EventType: "test.heartbeat", EntityType: "thing"}},
		Operators: []spec.Operator{{Name: "heartbeat_missing", Kind: "missing_heartbeat", Inputs: []string{"heartbeat"}, Duration: "5m", Output: "missing"}},
		Situation: spec.Situation{Type: "test", InitialPhase: "candidate", Phases: []spec.Phase{{Name: "candidate", Severity: 10}}, Occurrence: spec.Occurrence{OpenWhen: "features.missing == true"}, Reducers: []spec.Reducer{{Field: "facts.missing", Strategy: "latest_event_time", Input: "missing"}}},
	}
}

func appendLevel(t *testing.T, ctx context.Context, log *eventlog.EventLog, id string, offset time.Duration, level float64) int {
	t.Helper()
	when := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC).Add(offset)
	env := contractsv1.Envelope{ID: id, Type: "test.level", SchemaVersion: "1.0", TenantID: "default", Source: "test", PartitionKey: "thing-1", Entity: contractsv1.EntityRef{Type: "thing", ID: "thing-1"}, EventTime: when, IngestedAt: when, Classification: contractsv1.ClassificationInternal, Data: map[string]any{"level": level}}
	positions, err := log.Append(ctx, "default", []contractsv1.Envelope{env})
	if err != nil {
		t.Fatalf("append level: %v", err)
	}
	_ = positions
	return env.PartitionID(0)
}

func appendHeartbeat(t *testing.T, ctx context.Context, log *eventlog.EventLog, id string, offset time.Duration) int {
	t.Helper()
	when := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC).Add(offset)
	env := contractsv1.Envelope{ID: id, Type: "test.heartbeat", SchemaVersion: "1.0", TenantID: "default", Source: "test", PartitionKey: "thing-1", Entity: contractsv1.EntityRef{Type: "thing", ID: "thing-1"}, EventTime: when, IngestedAt: when, Classification: contractsv1.ClassificationInternal, Data: map[string]any{}}
	if _, err := log.Append(ctx, "default", []contractsv1.Envelope{env}); err != nil {
		t.Fatalf("append heartbeat: %v", err)
	}
	return env.PartitionID(0)
}
