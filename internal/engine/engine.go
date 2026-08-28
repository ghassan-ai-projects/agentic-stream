package engine

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
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
	mu           sync.Mutex
	db           *storage.DB
	log          *eventlog.EventLog
	clock        clock.Clock
	spec         *spec.CompiledSpec
	tenantID     string
	deploymentID string
	owner        *storage.RuntimeOwner
	ownerEpoch   string

	opRuntime *operators.OperatorRuntime
	sitEngine *situations.Engine
	cogEngine *cognition.Engine
}

// WithRuntimeOwner fences stream state transactions to the active runtime
// lease. It is used by live composition; deterministic replay leaves it unset.
func (e *Engine) WithRuntimeOwner(owner *storage.RuntimeOwner, epoch string) *Engine {
	e.owner = owner
	e.ownerEpoch = epoch
	return e
}

// NewEngine creates an engine for the given spec and tenant.
func NewEngine(ctx context.Context, db *storage.DB, log *eventlog.EventLog, clk clock.Clock, compiled *spec.CompiledSpec, tenantID string) (*Engine, error) {
	return newEngine(ctx, db, log, clk, compiled, tenantID, true)
}

// NewStreamEngine creates an engine that processes ingress, operators, and
// Situation versions without evaluating cognition. Replay uses this path for
// its deterministic stream projection.
func NewStreamEngine(ctx context.Context, db *storage.DB, log *eventlog.EventLog, clk clock.Clock, compiled *spec.CompiledSpec, tenantID string) (*Engine, error) {
	return newEngine(ctx, db, log, clk, compiled, tenantID, false)
}

func newEngine(ctx context.Context, db *storage.DB, log *eventlog.EventLog, clk clock.Clock, compiled *spec.CompiledSpec, tenantID string, cognitionEnabled bool) (*Engine, error) {
	if tenantID == "" {
		tenantID = contractsv1.TenantID
	}
	if err := spec.SaveDeployment(ctx, db, tenantID, compiled); err != nil {
		return nil, fmt.Errorf("save deployment: %w", err)
	}
	if len(compiled.Inputs) > 0 {
		requireSchemas := true
		for _, input := range compiled.Inputs {
			if input.SchemaRef == "" {
				requireSchemas = false
				break
			}
		}
		if requireSchemas {
			log.RequireSchemaValidation()
		}
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
	if err := restoreSituations(ctx, db, compiled.Digest, tenantID, sitEngine); err != nil {
		return nil, fmt.Errorf("restore situations: %w", err)
	}
	var cogEngine *cognition.Engine
	if cognitionEnabled {
		cogEngine, err = cognition.NewEngine(db, compiled.Digest, tenantID, compiled, idGen, clk)
		if err != nil {
			return nil, fmt.Errorf("cognition engine: %w", err)
		}
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

type persistedSituationState struct {
	SituationID    string               `json:"situation_id"`
	OccurrenceID   string               `json:"occurrence_id"`
	PartitionID    int                  `json:"partition_id"`
	Version        int                  `json:"version"`
	Facts          map[string]any       `json:"facts"`
	Evidence       []string             `json:"evidence"`
	ConditionStart map[string]time.Time `json:"condition_start"`
	Traceparent    string               `json:"traceparent,omitempty"`
	Tracestate     string               `json:"tracestate,omitempty"`
}

func restoreSituations(ctx context.Context, db *storage.DB, deploymentID, tenantID string, sitEngine *situations.Engine) error {
	rows, err := db.QueryContext(ctx, `
		SELECT s.situation_id, s.situation_type, s.entity_type, s.entity_id, s.partition_id,
		       s.occurrence_id, s.current_version, s.phase,
		       s.first_event_time, s.latest_event_time, s.updated_at,
		       s.state_codec_version, s.state_json, s.state_sha256,
		       v.previous_phase, v.severity, v.confidence,
		       v.completeness, v.traceparent, v.tracestate
		FROM situations s
		JOIN situation_versions v
		  ON v.situation_id = s.situation_id AND v.version = s.current_version
		WHERE s.deployment_id = ? AND s.tenant_id = ?
		ORDER BY s.partition_id, s.entity_type, s.entity_id`, deploymentID, tenantID)
	if err != nil {
		return fmt.Errorf("query current situations: %w", err)
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var situationID, situationType, entityType, entityID, occurrenceID, phase string
		var partitionID, version, severity int
		var firstEventTime, latestEventTime, updatedAt string
		var stateCodecVersion int
		var stateJSON, stateSHA256 []byte
		var previousPhase, traceparent, tracestate sql.NullString
		var confidence float64
		var completeness string
		if err := rows.Scan(&situationID, &situationType, &entityType, &entityID, &partitionID,
			&occurrenceID, &version, &phase, &firstEventTime, &latestEventTime,
			&updatedAt, &stateCodecVersion, &stateJSON, &stateSHA256, &previousPhase, &severity, &confidence,
			&completeness, &traceparent, &tracestate); err != nil {
			return fmt.Errorf("scan current situation: %w", err)
		}
		parseTime := func(name, value string) (time.Time, error) {
			parsed, err := time.Parse(time.RFC3339Nano, value)
			if err != nil {
				return time.Time{}, fmt.Errorf("parse %s: %w", name, err)
			}
			return parsed, nil
		}
		first, err := parseTime("first event time", firstEventTime)
		if err != nil {
			return err
		}
		latest, err := parseTime("latest event time", latestEventTime)
		if err != nil {
			return err
		}
		updated, err := parseTime("updated time", updatedAt)
		if err != nil {
			return err
		}
		var state persistedSituationState
		if stateCodecVersion == 0 {
			return fmt.Errorf("situation %s requires rebuild: legacy runtime state has no supported codec", situationID)
		}
		if stateCodecVersion != 1 {
			return fmt.Errorf("situation %s has unsupported state codec %d", situationID, stateCodecVersion)
		}
		if len(stateJSON) == 0 || len(stateSHA256) != sha256.Size {
			return fmt.Errorf("situation %s has incomplete persisted state", situationID)
		}
		var stateDocument map[string]any
		if err := json.Unmarshal(stateJSON, &stateDocument); err != nil {
			return fmt.Errorf("decode situation state document %s: %w", situationID, err)
		}
		stateDigest, err := canonicaljson.Digest(canonicaljson.DomainSituationState, stateDocument)
		if err != nil {
			return fmt.Errorf("digest situation state %s: %w", situationID, err)
		}
		decodedStateDigest, err := canonicaljson.DecodeDigest(stateDigest)
		if err != nil || !bytes.Equal(decodedStateDigest, stateSHA256) {
			return fmt.Errorf("situation %s persisted state digest mismatch", situationID)
		}
		if err := json.Unmarshal(stateJSON, &state); err != nil {
			return fmt.Errorf("decode situation state %s: %w", situationID, err)
		}
		if state.SituationID != situationID || state.OccurrenceID != occurrenceID || state.PartitionID != partitionID || state.Version != version {
			return fmt.Errorf("situation %s persisted state identity mismatch", situationID)
		}
		facts := state.Facts
		if facts == nil {
			facts = make(map[string]any)
		}
		for key, value := range facts {
			if !strings.HasSuffix(key, "_event_time") {
				continue
			}
			text, ok := value.(string)
			if !ok {
				continue
			}
			parsed, err := time.Parse(time.RFC3339Nano, text)
			if err != nil {
				return fmt.Errorf("parse fact time %s: %w", key, err)
			}
			facts[key] = parsed
		}
		evidence := make(map[string]struct{}, len(state.Evidence))
		for _, id := range state.Evidence {
			evidence[id] = struct{}{}
		}
		if state.Traceparent != "" {
			traceparent = sql.NullString{String: state.Traceparent, Valid: true}
		}
		if state.Tracestate != "" {
			tracestate = sql.NullString{String: state.Tracestate, Valid: true}
		}
		if err := sitEngine.Restore(situations.Situation{
			SituationID:     situationID,
			TenantID:        tenantID,
			DeploymentID:    deploymentID,
			Type:            situationType,
			EntityType:      entityType,
			EntityID:        entityID,
			PartitionID:     partitionID,
			OccurrenceID:    occurrenceID,
			Version:         version,
			Phase:           phase,
			PreviousPhase:   previousPhase.String,
			Severity:        severity,
			Confidence:      confidence,
			Completeness:    completeness,
			FirstEventTime:  first,
			LatestEventTime: latest,
			Facts:           facts,
			Evidence:        evidence,
			ConditionStart:  state.ConditionStart,
			OpenedAt:        first,
			UpdatedAt:       updated,
			Traceparent:     traceparent.String,
			Tracestate:      tracestate.String,
		}); err != nil {
			return fmt.Errorf("restore situation %s: %w", situationID, err)
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate current situations: %w", err)
	}
	return nil
}

// Run reads and applies events for partitionID until no more unprocessed
// records remain. It returns the number of events processed.
func (e *Engine) Run(ctx context.Context, partitionID int) (int, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.run(ctx, partitionID, nil)
}

// RunWithHook processes a partition and calls beforeApply immediately before
// each event is applied. The hook runs outside the engine transaction and is
// intended for deterministic replay clocks only.
func (e *Engine) RunWithHook(ctx context.Context, partitionID int, beforeApply func(eventlog.Record) error) (int, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.run(ctx, partitionID, beforeApply)
}

// RunGlobal applies all partitions in durable event-log position order. It is
// used by replay so one virtual clock cannot observe a later partition before
// an earlier record in the authoritative trace.
func (e *Engine) RunGlobal(ctx context.Context, beforeApply func(eventlog.Record) error) (int, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.runGlobal(ctx, beforeApply)
}

func (e *Engine) runGlobal(ctx context.Context, beforeApply func(eventlog.Record) error) (int, error) {
	var processed int
	var lastPosition eventlog.LogPosition
	for {
		var records []eventlog.Record
		if err := e.log.Read(ctx, eventlog.ReadRequest{TenantID: e.tenantID, PartitionID: -1, AfterPosition: lastPosition, Limit: 100}, func(record eventlog.Record) error {
			records = append(records, record)
			return nil
		}); err != nil {
			return processed, fmt.Errorf("read global event log: %w", err)
		}
		if len(records) == 0 {
			timerCount, err := e.runDueTimersForAllPartitions(ctx)
			if err != nil {
				return processed, fmt.Errorf("run global timers: %w", err)
			}
			if err := e.checkpointWAL(ctx); err != nil {
				return processed + timerCount, fmt.Errorf("checkpoint WAL after global timers: %w", err)
			}
			return processed + timerCount, nil
		}
		for _, record := range records {
			checkpoint, err := e.loadCheckpoint(ctx, record.PartitionID)
			if err != nil {
				return processed, fmt.Errorf("load partition checkpoint: %w", err)
			}
			watermark, err := e.watermarkForRecord(record.EventTime, checkpoint.Watermark)
			if err != nil {
				return processed, fmt.Errorf("watermark: %w", err)
			}
			if beforeApply != nil {
				if err := beforeApply(record); err != nil {
					return processed, fmt.Errorf("before apply hook: %w", err)
				}
			}
			fired, err := e.runDueTimersForAllPartitions(ctx)
			if err != nil {
				return processed, fmt.Errorf("run timers before event %d: %w", record.Position, err)
			}
			processed += fired
			if err := e.applyRecord(ctx, record.PartitionID, record, watermark); err != nil {
				return processed, fmt.Errorf("apply record %d: %w", record.Position, err)
			}
			lastPosition = record.Position
			processed++
		}
		if err := e.checkpointWAL(ctx); err != nil {
			return processed, fmt.Errorf("checkpoint WAL after global batch: %w", err)
		}
	}
}

func (e *Engine) runDueTimersForAllPartitions(ctx context.Context) (int, error) {
	rows, err := e.db.QueryContext(ctx, `
		SELECT DISTINCT partition_id FROM timers
		WHERE deployment_id = ? AND tenant_id = ? AND timer_kind = 'processing_time' AND status = 'pending'
		ORDER BY partition_id`, e.deploymentID, e.tenantID)
	if err != nil {
		return 0, fmt.Errorf("query timer partitions: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var partitions []int
	for rows.Next() {
		var partitionID int
		if err := rows.Scan(&partitionID); err != nil {
			return 0, fmt.Errorf("scan timer partition: %w", err)
		}
		partitions = append(partitions, partitionID)
	}
	if err := rows.Err(); err != nil {
		return 0, fmt.Errorf("iterate timer partitions: %w", err)
	}
	count := 0
	for _, partitionID := range partitions {
		fired, err := e.runDueTimers(ctx, partitionID)
		if err != nil {
			return count, err
		}
		count += fired
	}
	return count, nil
}

func (e *Engine) run(ctx context.Context, partitionID int, beforeApply func(eventlog.Record) error) (int, error) {
	processed := 0
	for {
		n, err := e.runBatch(ctx, partitionID, beforeApply)
		if err != nil {
			return processed, err
		}
		processed += n
		if n == 0 {
			break
		}
	}
	fired, err := e.runDueTimers(ctx, partitionID)
	if err != nil {
		return processed, err
	}
	processed += fired
	if err := e.checkpointWAL(ctx); err != nil {
		return processed, fmt.Errorf("checkpoint WAL after partition timers: %w", err)
	}
	return processed, nil
}

// RunDueTimers applies durable processing-time timers whose due time has
// passed according to the runtime clock. Timer-derived state commits in one
// transaction with the timer acknowledgement and Situation versions.
func (e *Engine) RunDueTimers(ctx context.Context, partitionID int) (int, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.runDueTimers(ctx, partitionID)
}

func (e *Engine) runDueTimers(ctx context.Context, partitionID int) (int, error) {
	now := e.clock.Now().UTC()
	checkpoint, err := e.loadCheckpoint(ctx, partitionID)
	if err != nil {
		return 0, fmt.Errorf("load timer checkpoint: %w", err)
	}
	watermark := now
	if checkpoint.Watermark != "" {
		watermark, err = time.Parse(time.RFC3339Nano, checkpoint.Watermark)
		if err != nil {
			return 0, fmt.Errorf("parse timer watermark: %w", err)
		}
	}

	fired := 0
	if err := e.db.WithTx(ctx, func(tx *sql.Tx) error {
		if err := e.assertOwner(ctx, tx); err != nil {
			return err
		}
		rows, err := tx.QueryContext(ctx, `
			SELECT timer_id, operator_id, state_key, due_at, payload_json FROM timers
			WHERE deployment_id = ? AND tenant_id = ? AND partition_id = ?
			  AND timer_kind = 'processing_time' AND status = 'pending' AND due_at <= ?
			ORDER BY due_at, timer_id`,
			e.deploymentID, e.tenantID, partitionID, now.Format(time.RFC3339Nano))
		if err != nil {
			return fmt.Errorf("query due timers: %w", err)
		}
		defer func() { _ = rows.Close() }()
		type timerInfo struct {
			id, operatorID, stateKey, dueAt, expectedEventID string
		}
		var timerInfos []timerInfo
		for rows.Next() {
			var timer timerInfo
			var payload []byte
			if err := rows.Scan(&timer.id, &timer.operatorID, &timer.stateKey, &timer.dueAt, &payload); err != nil {
				return fmt.Errorf("scan due timer: %w", err)
			}
			var timerPayload struct {
				ExpectedEventID string `json:"expected_event_id"`
			}
			if err := json.Unmarshal(payload, &timerPayload); err != nil {
				return fmt.Errorf("decode timer payload %s: %w", timer.id, err)
			}
			timer.expectedEventID = timerPayload.ExpectedEventID
			timerInfos = append(timerInfos, timer)
		}
		if err := rows.Err(); err != nil {
			return fmt.Errorf("iterate due timers: %w", err)
		}
		if len(timerInfos) == 0 {
			return nil
		}

		ps, err := e.loadOperatorStateForPartition(ctx, tx, partitionID)
		if err != nil {
			return err
		}
		features, _, err := e.opRuntime.ApplyTimer(ctx, ps, watermark, now)
		if err != nil {
			return fmt.Errorf("apply timers: %w", err)
		}
		dueByKey := make(map[string]timerInfo, len(timerInfos))
		for _, timer := range timerInfos {
			dueByKey[timer.operatorID+"\x00"+timer.stateKey] = timer
		}
		var appliedFeatures []operators.Feature
		matchedTimers := make(map[string]struct{}, len(timerInfos))
		for _, timer := range timerInfos {
			if !e.opRuntime.IsTimerStateActive(ps, timer.stateKey) {
				// Boot fencing intentionally suppresses this timer. Mark it
				// handled so a stale timer cannot deadlock the partition.
				matchedTimers[timer.id] = struct{}{}
			}
		}
		for _, feature := range features {
			stateKey := feature.StateKey
			if stateKey == "" {
				// Compatibility for features produced by older runtimes. New
				// timer-backed features always carry their exact durable key.
				stateKey = feature.EntityID
			}
			timer, ok := dueByKey[feature.OperatorID+"\x00"+stateKey]
			if !ok || len(feature.InputEventIDs) == 0 || feature.InputEventIDs[len(feature.InputEventIDs)-1] != timer.expectedEventID {
				continue
			}
			matchedTimers[timer.id] = struct{}{}
			feature.TenantID = e.tenantID
			feature.PartitionID = partitionID
			feature.Metadata = map[string]any{
				"timer_id":               timer.id,
				"timer_basis":            "processing_time",
				"timer_due_at":           timer.dueAt,
				"timer_fired_at":         now.Format(time.RFC3339Nano),
				"expected_event_horizon": timer.dueAt,
				"clock_quality":          clock.Quality(e.clock),
				"source_traceparent":     feature.Traceparent,
				"source_tracestate":      feature.Tracestate,
			}
			appliedFeatures = append(appliedFeatures, feature)
			versions, err := e.sitEngine.ApplyFeature(ctx, feature, watermark)
			if err != nil {
				return fmt.Errorf("apply timer situation: %w", err)
			}
			for _, version := range versions {
				if err := e.saveSituationVersion(ctx, tx, partitionID, version); err != nil {
					return fmt.Errorf("save timer situation version: %w", err)
				}
				if e.cogEngine != nil {
					if err := e.cogEngine.Process(ctx, tx, version); err != nil {
						return fmt.Errorf("process timer cognition: %w", err)
					}
				}
			}
		}
		if len(matchedTimers) != len(timerInfos) {
			return fmt.Errorf("due timer has no matching operator state: matched %d of %d", len(matchedTimers), len(timerInfos))
		}
		updatedSituations := make(map[string]struct{}, len(appliedFeatures))
		for _, feature := range appliedFeatures {
			key := feature.EntityType + "\x00" + feature.EntityID
			if _, seen := updatedSituations[key]; seen {
				continue
			}
			updatedSituations[key] = struct{}{}
			sit, stateJSON, stateDigest, ok, err := e.sitEngine.CurrentState(partitionID, feature.EntityType, feature.EntityID)
			if err != nil {
				return fmt.Errorf("snapshot current situation state: %w", err)
			}
			if ok && sit.Version > 0 {
				if err := e.saveSituationRuntimeState(ctx, tx, sit, stateJSON, stateDigest); err != nil {
					return fmt.Errorf("save current situation state: %w", err)
				}
			}
		}

		placeholders := strings.TrimRight(strings.Repeat("?,", len(timerInfos)), ",")
		args := make([]any, 0, len(timerInfos)+2)
		args = append(args, now.Format(time.RFC3339Nano))
		for _, timer := range timerInfos {
			args = append(args, timer.id)
		}
		args = append(args, e.tenantID, e.deploymentID)
		//nolint:gosec // placeholders are generated from timer count, never user input.
		query := fmt.Sprintf(`UPDATE timers SET status = 'fired', fired_at = ?
			WHERE timer_id IN (%s) AND tenant_id = ? AND deployment_id = ? AND status = 'pending'`, placeholders)
		if _, err := tx.ExecContext(ctx, query, args...); err != nil {
			return fmt.Errorf("acknowledge timers: %w", err)
		}
		fired = len(timerInfos)
		return nil
	}); err != nil {
		e.sitEngine.Reset()
		if restoreErr := restoreSituations(ctx, e.db, e.deploymentID, e.tenantID, e.sitEngine); restoreErr != nil {
			return 0, fmt.Errorf("run timers transaction: %w; restore after rollback: %w", err, restoreErr)
		}
		return 0, fmt.Errorf("run timers transaction: %w", err)
	}
	return fired, nil
}

// RunTimerLoop drives durable processing-time timers for a live runtime. The
// database remains authoritative; the clock timer is only a wake-up mechanism.
func (e *Engine) RunTimerLoop(ctx context.Context, pollInterval time.Duration) error {
	if pollInterval <= 0 {
		pollInterval = 100 * time.Millisecond
	}
	for {
		e.mu.Lock()
		_, err := e.runDueTimersForAllPartitions(ctx)
		e.mu.Unlock()
		if err != nil {
			return fmt.Errorf("drive timers: %w", err)
		}
		timer := e.clock.NewTimer(pollInterval)
		select {
		case <-ctx.Done():
			_ = timer.Stop()
			return fmt.Errorf("timer loop interrupted: %w", ctx.Err())
		case <-timer.C():
		}
	}
}

func (e *Engine) saveSituationRuntimeState(ctx context.Context, tx *sql.Tx, sit situations.Situation, stateJSON []byte, stateDigest string) error {
	stateDigestBytes, err := canonicaljson.DecodeDigest(stateDigest)
	if err != nil {
		return fmt.Errorf("invalid situation state digest: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE situations
		SET phase = ?, latest_event_time = ?, updated_at = ?, state_codec_version = 1,
		    state_json = ?, state_sha256 = ?
		WHERE situation_id = ? AND tenant_id = ? AND deployment_id = ? AND current_version = ?`,
		sit.Phase, sit.LatestEventTime.Format(time.RFC3339Nano), e.clock.Now().UTC().Format(time.RFC3339Nano),
		stateJSON, stateDigestBytes, sit.SituationID, e.tenantID, e.deploymentID, sit.Version); err != nil {
		return fmt.Errorf("update situation runtime state: %w", err)
	}
	return nil
}

func (e *Engine) runBatch(ctx context.Context, partitionID int, beforeApply func(eventlog.Record) error) (int, error) {
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
		if beforeApply != nil {
			if err := beforeApply(rec); err != nil {
				return 0, fmt.Errorf("before apply hook: %w", err)
			}
		}
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
	if err := e.checkpointWAL(ctx); err != nil {
		return 0, fmt.Errorf("checkpoint WAL after partition batch: %w", err)
	}

	return len(records), nil
}

func (e *Engine) checkpointWAL(ctx context.Context) error {
	if err := e.db.Checkpoint(ctx); err != nil && !storage.IsSQLiteBusy(err) {
		return fmt.Errorf("checkpoint WAL: %w", err)
	}
	return nil
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
	err := storage.RetrySQLiteBusy(ctx, func() error {
		err := e.db.WithTx(ctx, func(tx *sql.Tx) error {
			if err := e.assertOwner(ctx, tx); err != nil {
				return err
			}
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

			features, newPS, err := e.opRuntime.ApplyEventAt(ctx, ps, rec.Envelope, watermark, e.clock.Now().UTC())
			if err != nil {
				return fmt.Errorf("apply operators: %w", err)
			}
			affected := make(map[string]struct{})

			for _, feature := range features {
				affected[feature.EntityType+"\x00"+feature.EntityID] = struct{}{}
				feature.TenantID = e.tenantID
				feature.PartitionID = partitionID
				versions, err := e.sitEngine.ApplyFeature(ctx, feature, watermark)
				if err != nil {
					return fmt.Errorf("apply situation: %w", err)
				}
				for _, v := range versions {
					if err := e.saveSituationVersion(ctx, tx, partitionID, v); err != nil {
						return fmt.Errorf("save situation version: %w", err)
					}
					if e.cogEngine != nil {
						if err := e.cogEngine.Process(ctx, tx, v); err != nil {
							return fmt.Errorf("cognition process: %w", err)
						}
					}
				}
			}
			for key := range affected {
				parts := strings.SplitN(key, "\x00", 2)
				sit, stateJSON, stateDigest, ok, err := e.sitEngine.CurrentState(partitionID, parts[0], parts[1])
				if err != nil {
					return fmt.Errorf("snapshot current situation state: %w", err)
				}
				if ok && sit.Version > 0 {
					if err := e.saveSituationRuntimeState(ctx, tx, sit, stateJSON, stateDigest); err != nil {
						return fmt.Errorf("save current situation state: %w", err)
					}
				}
			}

			if err := e.saveOperatorState(ctx, tx, partitionID, rec.Envelope.Entity.ID, newPS); err != nil {
				return fmt.Errorf("save operator state: %w", err)
			}
			if err := e.scheduleHeartbeatTimers(ctx, tx, partitionID, newPS); err != nil {
				return fmt.Errorf("schedule heartbeat timers: %w", err)
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
		})
		if err == nil {
			return nil
		}

		e.sitEngine.Reset()
		if restoreErr := restoreSituations(ctx, e.db, e.deploymentID, e.tenantID, e.sitEngine); restoreErr != nil {
			return fmt.Errorf("apply record transaction: %w; restore after rollback: %w", err, restoreErr)
		}
		return fmt.Errorf("apply transaction: %w", err)
	})
	if err != nil {
		return fmt.Errorf("apply record transaction: %w", err)
	}
	return nil
}

func (e *Engine) assertOwner(ctx context.Context, tx *sql.Tx) error {
	if e.owner == nil || e.ownerEpoch == "" {
		return nil
	}
	if err := e.owner.Assert(ctx, tx, e.ownerEpoch); err != nil {
		return fmt.Errorf("stream runtime ownership lost: %w", err)
	}
	return nil
}

func (e *Engine) loadOperatorStateForPartition(ctx context.Context, tx *sql.Tx, partitionID int) (*operators.PartitionState, error) {
	ps := &operators.PartitionState{OperatorStates: make(map[string]map[string]*operators.OperatorStateBlob)}
	rows, err := tx.QueryContext(ctx, `
		SELECT operator_id, state_key, state_blob FROM operator_state
		WHERE deployment_id = ? AND tenant_id = ? AND partition_id = ?`,
		e.deploymentID, e.tenantID, partitionID)
	if err != nil {
		return nil, fmt.Errorf("query partition operator state: %w", err)
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var operatorID, stateKey string
		var blobJSON []byte
		if err := rows.Scan(&operatorID, &stateKey, &blobJSON); err != nil {
			return nil, fmt.Errorf("scan partition operator state: %w", err)
		}
		var blob operators.OperatorStateBlob
		if err := json.Unmarshal(blobJSON, &blob); err != nil {
			return nil, fmt.Errorf("unmarshal partition operator state: %w", err)
		}
		if ps.OperatorStates[operatorID] == nil {
			ps.OperatorStates[operatorID] = make(map[string]*operators.OperatorStateBlob)
		}
		ps.OperatorStates[operatorID][stateKey] = &blob
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate partition operator state: %w", err)
	}
	return ps, nil
}

func (e *Engine) scheduleHeartbeatTimers(ctx context.Context, tx *sql.Tx, partitionID int, ps *operators.PartitionState) error {
	if ps == nil {
		return nil
	}
	now := e.clock.Now().UTC().Format(time.RFC3339Nano)
	for _, op := range e.spec.Operators {
		if op.Kind != "missing_heartbeat" {
			continue
		}
		duration, err := duration.Parse(op.Duration)
		if err != nil {
			return fmt.Errorf("parse %s duration: %w", op.Name, err)
		}
		for stateKey, blob := range ps.OperatorStates[op.Name] {
			if blob == nil || blob.Heartbeat == nil || blob.Heartbeat.LastEventTime == nil {
				continue
			}
			processingTime := blob.Heartbeat.LastProcessingTime
			if processingTime == nil {
				processingTime = blob.Heartbeat.LastEventTime
			}
			dueAt := processingTime.Add(duration).UTC()
			if _, err := tx.ExecContext(ctx, `
				UPDATE timers SET status = 'cancelled'
				WHERE deployment_id = ? AND tenant_id = ? AND partition_id = ?
				  AND operator_id = ? AND state_key = ? AND timer_kind = 'processing_time' AND status = 'pending'`,
				e.deploymentID, e.tenantID, partitionID, op.Name, stateKey); err != nil {
				return fmt.Errorf("cancel prior heartbeat timer: %w", err)
			}
			payload, err := json.Marshal(map[string]any{
				"operator_id":       op.Name,
				"state_key":         stateKey,
				"expected_event_id": blob.Heartbeat.LastEventID,
				"due_at":            dueAt.Format(time.RFC3339Nano),
			})
			if err != nil {
				return fmt.Errorf("marshal heartbeat timer: %w", err)
			}
			h := sha256.Sum256([]byte(fmt.Sprintf("agentic-stream/timer/v1\x00%s\x00%s\x00%d\x00%s\x00%s\x00%s\x00%s",
				e.deploymentID, e.tenantID, partitionID, op.Name, stateKey, "processing_time", dueAt.Format(time.RFC3339Nano))))
			timerID := "tmr_" + hex.EncodeToString(h[:])
			if _, err := tx.ExecContext(ctx, `
				INSERT INTO timers (
					timer_id, deployment_id, tenant_id, partition_id, operator_id, state_key,
					timer_kind, due_at, payload_json, status, created_at
				) VALUES (?, ?, ?, ?, ?, ?, 'processing_time', ?, ?, 'pending', ?)
				ON CONFLICT(deployment_id, tenant_id, partition_id, operator_id, state_key, timer_kind, due_at)
				DO UPDATE SET status = 'pending', payload_json = excluded.payload_json
				WHERE timers.status != 'fired'`,
				timerID, e.deploymentID, e.tenantID, partitionID, op.Name, stateKey,
				dueAt.Format(time.RFC3339Nano), payload, now); err != nil {
				return fmt.Errorf("insert heartbeat timer: %w", err)
			}
		}
	}
	return nil
}

func (e *Engine) loadOperatorState(ctx context.Context, tx *sql.Tx, partitionID int, entityID string) (*operators.PartitionState, error) {
	ps := &operators.PartitionState{OperatorStates: make(map[string]map[string]*operators.OperatorStateBlob)}
	rows, err := tx.QueryContext(ctx,
		`SELECT operator_id, state_key, state_blob FROM operator_state
		 WHERE deployment_id = ? AND tenant_id = ? AND partition_id = ?
		   AND (state_key = ? OR (length(state_key) > length(?) AND
		        substr(state_key, 1, length(?) + 1) = ? || char(31)))`,
		e.deploymentID, e.tenantID, partitionID, entityID, entityID, entityID, entityID,
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
	if _, err := tx.ExecContext(ctx, `
		DELETE FROM operator_state
		WHERE deployment_id = ? AND tenant_id = ? AND partition_id = ?
		  AND (state_key = ? OR (length(state_key) > length(?) AND
		       substr(state_key, 1, length(?) + 1) = ? || char(31)))`,
		e.deploymentID, e.tenantID, partitionID, entityID, entityID, entityID, entityID,
	); err != nil {
		return fmt.Errorf("retire prior operator state: %w", err)
	}
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
	firstEventTime := v.FirstEventTime
	if firstEventTime.IsZero() {
		firstEventTime = v.EventHorizon
	}
	occurrenceID := v.OccurrenceID
	if occurrenceID == "" {
		occurrenceID = "occ-" + v.SituationID
	}
	stateJSON := v.StateJSON
	if len(stateJSON) == 0 {
		return fmt.Errorf("situation %s version %d has no runtime state", v.SituationID, v.Version)
	}
	stateDigest := v.StateSHA256
	if stateDigest == "" {
		var stateDocument map[string]any
		if err := json.Unmarshal(stateJSON, &stateDocument); err != nil {
			return fmt.Errorf("decode situation state for digest: %w", err)
		}
		var err error
		stateDigest, err = canonicaljson.Digest(canonicaljson.DomainSituationState, stateDocument)
		if err != nil {
			return fmt.Errorf("digest situation state: %w", err)
		}
	}
	stateDigestBytes, err := canonicaljson.DecodeDigest(stateDigest)
	if err != nil {
		return fmt.Errorf("invalid situation state digest: %w", err)
	}

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
			status, first_event_time, latest_event_time, updated_at, created_at,
			state_codec_version, state_json, state_sha256
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(situation_id)
		DO UPDATE SET current_version = excluded.current_version,
		              phase = excluded.phase,
		              status = excluded.status,
		              latest_event_time = excluded.latest_event_time,
		              updated_at = excluded.updated_at,
		              state_codec_version = excluded.state_codec_version,
		              state_json = excluded.state_json,
		              state_sha256 = excluded.state_sha256`,
		v.SituationID, e.tenantID, e.deploymentID, v.Type, v.EntityType,
		v.EntityID, partitionID, occurrenceID, v.Version, v.Phase,
		"active", firstEventTime.Format(time.RFC3339Nano),
		v.EventHorizon.Format(time.RFC3339Nano), now, now, 1, stateJSON, stateDigestBytes,
	); err != nil {
		return fmt.Errorf("upsert situation: %w", err)
	}
	snapshotDigestBytes, err := canonicaljson.DecodeDigest(v.SnapshotSHA256)
	if err != nil {
		return fmt.Errorf("invalid snapshot digest: %w", err)
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
			valid_from, snapshot_json, snapshot_sha256, lineage_id, traceparent, tracestate, created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		v.SituationID, v.Version, prevVersion, v.Phase, v.PreviousPhase,
		v.Severity, v.Confidence, v.Completeness,
		v.EventHorizon.Format(time.RFC3339Nano), v.Watermark.Format(time.RFC3339Nano),
		v.EventHorizon.Format(time.RFC3339Nano), v.SnapshotJSON, snapshotDigestBytes,
		lineageID, nullableString(v.Traceparent), nullableString(v.Tracestate), now,
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

func nullableString(value string) sql.NullString {
	return sql.NullString{String: value, Valid: value != ""}
}
