package episodes

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"

	"github.com/ghassan-ai-projects/agentic-stream/internal/contractsv1"
	domain "github.com/ghassan-ai-projects/agentic-stream/internal/episodes/internal/domain"
)

// assemblyInputs are the durable facts one scheduler item is assembled from.
type assemblyInputs struct {
	evaluation      evaluation
	snapshot        *domain.SnapshotEvidence
	delta           map[string]any
	reconsideration map[string]any
}

type schedulerItem struct {
	SchedulerItemID  string
	Kind             string
	TriggerID        string
	TenantID         string
	SituationID      string
	SituationVersion int
}

type evaluation struct {
	TriggerID   string
	TriggerName string
	Score       float64
	Threshold   float64
	Lane        string
	DeltaJSON   []byte
}

// loadInputs reads the trigger evaluation, the validated Situation snapshot,
// and, for a reconsider item, the reconsideration context.
func (a *Assembler) loadInputs(ctx context.Context, tx *sql.Tx, item schedulerItem, tenantID string) (assemblyInputs, error) {
	var inputs assemblyInputs
	var err error
	inputs.evaluation, err = a.loadEvaluation(ctx, tx, item.TriggerID)
	if err != nil {
		return assemblyInputs{}, fmt.Errorf("load evaluation: %w", err)
	}
	inputs.snapshot, err = a.loadValidatedSnapshot(ctx, tx, item.SituationID, item.SituationVersion, tenantID)
	if err != nil {
		return assemblyInputs{}, err
	}
	return loadTriggerContext(ctx, tx, item, inputs)
}

func (a *Assembler) loadEvaluation(ctx context.Context, tx *sql.Tx, triggerID string) (evaluation, error) {
	var ev evaluation
	if err := tx.QueryRowContext(ctx, `
		SELECT trigger_id, trigger_name, score, threshold, lane, delta_json
		FROM trigger_evaluations WHERE trigger_id = ?`,
		triggerID,
	).Scan(&ev.TriggerID, &ev.TriggerName, &ev.Score, &ev.Threshold, &ev.Lane, &ev.DeltaJSON); err != nil {
		return ev, fmt.Errorf("query evaluation: %w", err)
	}
	return ev, nil
}

// loadValidatedSnapshot loads a situation snapshot for a version and validates
// it — the single guard both Assemble and Rebind use so a snapshot can never
// reach a worker unvalidated or unbound to its persisted digest.
func (a *Assembler) loadValidatedSnapshot(ctx context.Context, tx *sql.Tx, situationID string, version int, tenantID string) (*domain.SnapshotEvidence, error) {
	snapshotJSON, persistedDigest, traceparent, tracestate, err := a.loadSnapshotJSON(ctx, tx, situationID, version)
	if err != nil {
		return nil, fmt.Errorf("load snapshot: %w", err)
	}
	return domain.ValidateSnapshotEvidence(snapshotJSON, persistedDigest, traceparent, tracestate, situationID, version, tenantID)
}

func (a *Assembler) loadSnapshotJSON(ctx context.Context, tx *sql.Tx, situationID string, version int) ([]byte, []byte, string, string, error) {
	var snapshotJSON []byte
	var snapshotDigest []byte
	var traceparent, tracestate sql.NullString
	if err := tx.QueryRowContext(ctx, `
		SELECT snapshot_json, snapshot_sha256, traceparent, tracestate FROM situation_versions
		WHERE situation_id = ? AND version = ?`,
		situationID, version).Scan(&snapshotJSON, &snapshotDigest, &traceparent, &tracestate); err != nil {
		return nil, nil, "", "", fmt.Errorf("query situation version: %w", err)
	}
	if _, err := contractsv1.ParseTraceContext(traceparent.String, tracestate.String); err != nil {
		return nil, nil, "", "", fmt.Errorf("validate situation trace context: %w", err)
	}
	return snapshotJSON, snapshotDigest, traceparent.String, tracestate.String, nil
}

func loadTriggerContext(ctx context.Context, tx *sql.Tx, item schedulerItem, inputs assemblyInputs) (assemblyInputs, error) {
	var err error
	if len(inputs.evaluation.DeltaJSON) > 0 {
		if err := json.Unmarshal(inputs.evaluation.DeltaJSON, &inputs.delta); err != nil {
			return assemblyInputs{}, fmt.Errorf("unmarshal delta: %w", err)
		}
	}
	if item.Kind == "reconsider" {
		inputs.reconsideration, err = loadReconsideration(ctx, tx, item, inputs.evaluation, inputs.delta, inputs.snapshot.Document)
		if err != nil {
			return assemblyInputs{}, fmt.Errorf("load reconsideration: %w", err)
		}
	}
	return inputs, nil
}

func requestEntityID(raw []byte) (string, error) {
	var request struct {
		Snapshot json.RawMessage `json:"snapshot"`
	}
	if err := json.Unmarshal(raw, &request); err != nil {
		return "", fmt.Errorf("decode request snapshot: %w", err)
	}
	if len(request.Snapshot) == 0 {
		return "", nil
	}
	return requestSnapshotEntity(request.Snapshot)
}

func requestSnapshotEntity(raw []byte) (string, error) {
	var snapshot struct {
		Entity struct {
			ID string `json:"id"`
		} `json:"entity"`
	}
	if err := json.Unmarshal(raw, &snapshot); err != nil {
		return "", fmt.Errorf("decode request snapshot entity: %w", err)
	}
	return snapshot.Entity.ID, nil
}

func (a *Assembler) loadSchedulerItem(ctx context.Context, tx *sql.Tx, id string) (schedulerItem, error) {
	var item schedulerItem
	if err := tx.QueryRowContext(ctx, `
		SELECT scheduler_item_id, kind, trigger_id, tenant_id, situation_id, situation_version
		FROM scheduler_items WHERE scheduler_item_id = ?`,
		id,
	).Scan(&item.SchedulerItemID, &item.Kind, &item.TriggerID, &item.TenantID, &item.SituationID, &item.SituationVersion); err != nil {
		return item, fmt.Errorf("query scheduler item: %w", err)
	}
	return item, nil
}
