package engine

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/canonicaljson"
	"github.com/ghassan-ai-projects/agentic-stream/internal/clock"
	"github.com/ghassan-ai-projects/agentic-stream/internal/cognition"
	"github.com/ghassan-ai-projects/agentic-stream/internal/contractsv1"
	"github.com/ghassan-ai-projects/agentic-stream/internal/duration"
	"github.com/ghassan-ai-projects/agentic-stream/internal/eventlog"
	"github.com/ghassan-ai-projects/agentic-stream/internal/ids"
	"github.com/ghassan-ai-projects/agentic-stream/internal/operators"
	"github.com/ghassan-ai-projects/agentic-stream/internal/situations"
	"github.com/ghassan-ai-projects/agentic-stream/internal/spec"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
)

// ConsumerName identifies the engine's inbox consumer.
const ConsumerName = "engine"

// Engine processes events for a single virtual partition deterministically.
type Engine struct {
	db           *storage.DB
	log          *eventlog.EventLog
	clock        clock.Clock
	spec         *spec.CompiledSpec
	tenantID     string
	deploymentID string

	opRuntime *operators.OperatorRuntime
	sitEngine *situations.Engine
	cogEngine *cognition.Engine
}

// NewEngine creates an engine for the given spec and tenant.
func NewEngine(ctx context.Context, db *storage.DB, log *eventlog.EventLog, clk clock.Clock, compiled *spec.CompiledSpec, tenantID string) (*Engine, error) {
	if tenantID == "" {
		tenantID = contractsv1.TenantID
	}
	if err := spec.SaveDeployment(ctx, db, tenantID, compiled); err != nil {
		return nil, fmt.Errorf("save deployment: %w", err)
	}
	idGen := ids.Deterministic()
	opRuntime, err := operators.NewOperatorRuntime(compiled.Digest, compiled, idGen)
	if err != nil {
		return nil, fmt.Errorf("operator runtime: %w", err)
	}
	// Partition ID is resolved per event; 0 is used only for stateless setup.
	sitEngine, err := situations.NewEngine(compiled.Digest, tenantID, 0, compiled, idGen)
	if err != nil {
		return nil, fmt.Errorf("situation engine: %w", err)
	}
	cogEngine, err := cognition.NewEngine(db, compiled.Digest, tenantID, compiled, idGen, clk)
	if err != nil {
		return nil, fmt.Errorf("cognition engine: %w", err)
	}
	return &Engine{
		db:           db,
		log:          log,
		clock:        clk,
		spec:         compiled,
		tenantID:     tenantID,
		deploymentID: compiled.Digest,
		opRuntime:    opRuntime,
		sitEngine:    sitEngine,
		cogEngine:    cogEngine,
	}, nil
}

// Run reads and applies events for partitionID until no more unprocessed
// records remain. It returns the number of events processed.
func (e *Engine) Run(ctx context.Context, partitionID int) (int, error) {
	processed := 0
	for {
		n, err := e.runBatch(ctx, partitionID)
		if err != nil {
			return processed, err
		}
		processed += n
		if n == 0 {
			break
		}
	}
	return processed, nil
}

func (e *Engine) runBatch(ctx context.Context, partitionID int) (int, error) {
	checkpoint, err := e.loadCheckpoint(ctx, partitionID)
	if err != nil {
		return 0, fmt.Errorf("load checkpoint: %w", err)
	}

	const batchSize = 100
	var records []eventlog.Record
	err = e.log.Read(ctx, eventlog.ReadRequest{
		TenantID:      e.tenantID,
		PartitionID:   partitionID,
		AfterPosition: checkpoint.LastPosition,
		Limit:         batchSize,
	}, func(r eventlog.Record) error {
		records = append(records, r)
		return nil
	})
	if err != nil {
		return 0, fmt.Errorf("read event log: %w", err)
	}
	if len(records) == 0 {
		return 0, nil
	}

	for _, rec := range records {
		watermark, err := e.watermarkForRecord(rec.EventTime, checkpoint.Watermark)
		if err != nil {
			return 0, fmt.Errorf("watermark: %w", err)
		}
		if err := e.applyRecord(ctx, partitionID, rec, watermark); err != nil {
			return 0, fmt.Errorf("apply record %d: %w", rec.Position, err)
		}
		checkpoint.LastPosition = rec.Position
		checkpoint.Watermark = watermark.Format(time.RFC3339Nano)
	}

	return len(records), nil
}

type checkpoint struct {
	LastPosition eventlog.LogPosition
	Watermark    string
}

func (e *Engine) loadCheckpoint(ctx context.Context, partitionID int) (checkpoint, error) {
	var cp checkpoint
	var lastPosition int64
	var watermark sql.NullString
	if err := e.db.QueryRowContext(ctx,
		"SELECT last_position, watermark FROM partition_checkpoints WHERE consumer_name = ? AND tenant_id = ? AND partition_id = ?",
		ConsumerName, e.tenantID, partitionID,
	).Scan(&lastPosition, &watermark); err != nil && err != sql.ErrNoRows {
		return cp, fmt.Errorf("query checkpoint: %w", err)
	}
	cp.LastPosition = eventlog.LogPosition(lastPosition)
	if watermark.Valid {
		cp.Watermark = watermark.String
	}
	return cp, nil
}

func (e *Engine) watermarkForRecord(eventTime time.Time, prevWatermark string) (time.Time, error) {
	maxLag, err := duration.Parse(e.spec.Time.MaxOutOfOrderness)
	if err != nil {
		return time.Time{}, fmt.Errorf("parse maxOutOfOrderness: %w", err)
	}
	wm := eventTime.Add(-maxLag)
	if prevWatermark != "" {
		prev, err := time.Parse(time.RFC3339Nano, prevWatermark)
		if err != nil {
			return time.Time{}, fmt.Errorf("parse prev watermark: %w", err)
		}
		if wm.Before(prev) {
			return prev, nil
		}
	}
	return wm, nil
}

func (e *Engine) applyRecord(ctx context.Context, partitionID int, rec eventlog.Record, watermark time.Time) error {
	if err := e.db.WithTx(ctx, func(tx *sql.Tx) error {
		// Idempotency: skip if already applied.
		var applied bool
		if err := tx.QueryRowContext(ctx,
			"SELECT 1 FROM event_inbox WHERE consumer_name = ? AND tenant_id = ? AND event_id = ?",
			ConsumerName, e.tenantID, rec.EventID,
		).Scan(&applied); err != nil && err != sql.ErrNoRows {
			return fmt.Errorf("check inbox: %w", err)
		}
		if applied {
			return nil
		}

		ps, err := e.loadOperatorState(ctx, tx, partitionID, rec.Envelope.Entity.ID)
		if err != nil {
			return fmt.Errorf("load operator state: %w", err)
		}

		features, newPS, err := e.opRuntime.ApplyEvent(ctx, ps, rec.Envelope, watermark)
		if err != nil {
			return fmt.Errorf("apply operators: %w", err)
		}

		for _, feature := range features {
			versions, err := e.sitEngine.ApplyFeature(ctx, feature, watermark)
			if err != nil {
				return fmt.Errorf("apply situation: %w", err)
			}
			for _, v := range versions {
				if err := e.saveSituationVersion(ctx, tx, partitionID, v); err != nil {
					return fmt.Errorf("save situation version: %w", err)
				}
				if err := e.cogEngine.Process(ctx, tx, v); err != nil {
					return fmt.Errorf("cognition process: %w", err)
				}
			}
		}

		if err := e.saveOperatorState(ctx, tx, partitionID, rec.Envelope.Entity.ID, newPS); err != nil {
			return fmt.Errorf("save operator state: %w", err)
		}

		now := e.clock.Now().UTC().Format(time.RFC3339Nano)

		// Advance checkpoint.
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO partition_checkpoints (consumer_name, tenant_id, partition_id, last_position, watermark, updated_at)
			VALUES (?, ?, ?, ?, ?, ?)
			ON CONFLICT(consumer_name, tenant_id, partition_id)
			DO UPDATE SET last_position = excluded.last_position, watermark = excluded.watermark, updated_at = excluded.updated_at`,
			ConsumerName, e.tenantID, partitionID, int64(rec.Position), watermark.Format(time.RFC3339Nano), now,
		); err != nil {
			return fmt.Errorf("update checkpoint: %w", err)
		}

		// Mark inbox applied.
		if _, err := tx.ExecContext(ctx,
			"INSERT INTO event_inbox (consumer_name, tenant_id, event_id, log_position, applied_at) VALUES (?, ?, ?, ?, ?)",
			ConsumerName, e.tenantID, rec.EventID, int64(rec.Position), now,
		); err != nil {
			return fmt.Errorf("mark inbox: %w", err)
		}

		return nil
	}); err != nil {
		return fmt.Errorf("apply record transaction: %w", err)
	}
	return nil
}

func (e *Engine) loadOperatorState(ctx context.Context, tx *sql.Tx, partitionID int, entityID string) (*operators.PartitionState, error) {
	ps := &operators.PartitionState{OperatorStates: make(map[string]map[string]*operators.OperatorStateBlob)}
	rows, err := tx.QueryContext(ctx,
		`SELECT operator_id, state_key, state_blob FROM operator_state
		 WHERE deployment_id = ? AND tenant_id = ? AND partition_id = ? AND state_key = ?`,
		e.deploymentID, e.tenantID, partitionID, entityID,
	)
	if err != nil {
		return nil, fmt.Errorf("query operator state: %w", err)
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var operatorID, stateKey string
		var blobJSON []byte
		if err := rows.Scan(&operatorID, &stateKey, &blobJSON); err != nil {
			return nil, fmt.Errorf("scan operator state: %w", err)
		}
		var blob operators.OperatorStateBlob
		if err := json.Unmarshal(blobJSON, &blob); err != nil {
			return nil, fmt.Errorf("unmarshal operator state: %w", err)
		}
		if ps.OperatorStates[operatorID] == nil {
			ps.OperatorStates[operatorID] = make(map[string]*operators.OperatorStateBlob)
		}
		ps.OperatorStates[operatorID][stateKey] = &blob
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate operator state: %w", err)
	}
	return ps, nil
}

func (e *Engine) saveOperatorState(ctx context.Context, tx *sql.Tx, partitionID int, entityID string, ps *operators.PartitionState) error {
	if ps == nil {
		return nil
	}
	now := e.clock.Now().UTC().Format(time.RFC3339Nano)
	for operatorID, keys := range ps.OperatorStates {
		for stateKey, blob := range keys {
			blobJSON, err := json.Marshal(blob)
			if err != nil {
				return fmt.Errorf("marshal operator state: %w", err)
			}
			h := sha256.Sum256(blobJSON)
			if _, err := tx.ExecContext(ctx, `
				INSERT INTO operator_state (
					deployment_id, tenant_id, partition_id, operator_id, state_key,
					state_version, codec_version, state_blob, state_sha256, updated_at
				) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
				ON CONFLICT(deployment_id, tenant_id, partition_id, operator_id, state_key)
				DO UPDATE SET state_version = excluded.state_version + 1,
				              state_blob = excluded.state_blob,
				              state_sha256 = excluded.state_sha256,
				              updated_at = excluded.updated_at`,
				e.deploymentID, e.tenantID, partitionID, operatorID, stateKey,
				1, 1, blobJSON, h[:], now,
			); err != nil {
				return fmt.Errorf("upsert operator state: %w", err)
			}
		}
	}
	return nil
}

func (e *Engine) saveSituationVersion(ctx context.Context, tx *sql.Tx, partitionID int, v situations.Version) error {
	now := e.clock.Now().UTC().Format(time.RFC3339Nano)

	// Ensure the lineage set exists.
	lineageID := e.lineageID(v.Evidence)
	referencesJSON, err := json.Marshal(v.Evidence)
	if err != nil {
		return fmt.Errorf("marshal evidence: %w", err)
	}
	lh := sha256.Sum256(referencesJSON)
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO lineage_sets (lineage_id, sha256, reference_count, references_json, created_at)
		VALUES (?, ?, 1, ?, ?)
		ON CONFLICT(lineage_id) DO NOTHING`,
		lineageID, lh[:], referencesJSON, now,
	); err != nil {
		return fmt.Errorf("insert lineage set: %w", err)
	}

	// Upsert current situation projection.
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO situations (
			situation_id, tenant_id, deployment_id, situation_type, entity_type,
			entity_id, partition_id, occurrence_id, current_version, phase,
			status, first_event_time, latest_event_time, updated_at, created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(situation_id)
		DO UPDATE SET current_version = excluded.current_version,
		              phase = excluded.phase,
		              status = excluded.status,
		              latest_event_time = excluded.latest_event_time,
		              updated_at = excluded.updated_at`,
		v.SituationID, e.tenantID, e.deploymentID, v.Type, v.EntityType,
		v.EntityID, partitionID, "occ-"+v.SituationID, v.Version, v.Phase,
		"active", v.EventHorizon.Format(time.RFC3339Nano),
		v.EventHorizon.Format(time.RFC3339Nano), now, now,
	); err != nil {
		return fmt.Errorf("upsert situation: %w", err)
	}

	// Insert immutable version.
	var prevVersion sql.NullInt64
	if v.PreviousVersion >= 1 {
		prevVersion = sql.NullInt64{Int64: int64(v.PreviousVersion), Valid: true}
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO situation_versions (
			situation_id, version, previous_version, phase, previous_phase,
			severity, confidence, completeness, event_horizon, watermark,
			valid_from, snapshot_json, snapshot_sha256, lineage_id, created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		v.SituationID, v.Version, prevVersion, v.Phase, v.PreviousPhase,
		v.Severity, v.Confidence, v.Completeness,
		v.EventHorizon.Format(time.RFC3339Nano), v.Watermark.Format(time.RFC3339Nano),
		v.EventHorizon.Format(time.RFC3339Nano), v.SnapshotJSON, mustDecodeHex(v.SnapshotSHA256),
		lineageID, now,
	); err != nil {
		return fmt.Errorf("insert situation version: %w", err)
	}
	return nil
}

func (e *Engine) lineageID(evidence []string) string {
	// Simple content-addressed lineage for Phase 2.
	h := sha256.New()
	for _, id := range evidence {
		_, _ = h.Write([]byte(id))
	}
	return "lin_" + hex.EncodeToString(h.Sum(nil))
}

func mustDecodeHex(s string) []byte {
	b, err := canonicaljson.DecodeDigest(s)
	if err != nil {
		panic(err)
	}
	return b
}
