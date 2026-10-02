package episodes

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"

	"github.com/ghassan-ai-projects/agentic-stream/internal/canonicaljson"
	"github.com/ghassan-ai-projects/agentic-stream/internal/contractsv1"
)

// assemblyInputs are the durable facts one scheduler item is assembled from.
type assemblyInputs struct {
	evaluation      evaluation
	snapshot        *snapshotEvidence
	delta           map[string]any
	reconsideration map[string]any
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
	if len(inputs.evaluation.DeltaJSON) > 0 {
		if err := json.Unmarshal(inputs.evaluation.DeltaJSON, &inputs.delta); err != nil {
			return assemblyInputs{}, fmt.Errorf("unmarshal delta: %w", err)
		}
	}
	if item.Kind == "reconsider" {
		inputs.reconsideration, err = loadReconsideration(ctx, tx, item, inputs.evaluation, inputs.delta, inputs.snapshot.document)
		if err != nil {
			return assemblyInputs{}, fmt.Errorf("load reconsideration: %w", err)
		}
	}
	return inputs, nil
}

func snapshotEntityID(raw []byte) (string, error) {
	var snapshot struct {
		Entity struct {
			ID string `json:"id"`
		} `json:"entity"`
	}
	if err := json.Unmarshal(raw, &snapshot); err != nil {
		return "", fmt.Errorf("decode snapshot entity: %w", err)
	}
	if snapshot.Entity.ID == "" {
		return "", fmt.Errorf("snapshot entity id is required")
	}
	return snapshot.Entity.ID, nil
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
	var snapshot struct {
		Entity struct {
			ID string `json:"id"`
		} `json:"entity"`
	}
	if err := json.Unmarshal(request.Snapshot, &snapshot); err != nil {
		return "", fmt.Errorf("decode request snapshot entity: %w", err)
	}
	return snapshot.Entity.ID, nil
}

// snapshotEvidence is a situation snapshot validated against the persisted
// version row: schema, identity, entity, and the stored snapshot digest.
type snapshotEvidence struct {
	json        []byte
	document    map[string]any
	entityID    string
	digest      string
	traceparent string
	tracestate  string
}

// loadValidatedSnapshot loads a situation snapshot for a version and validates
// it — the single guard both Assemble and Rebind use so a snapshot can never
// reach a worker unvalidated or unbound to its persisted digest.
func (a *Assembler) loadValidatedSnapshot(ctx context.Context, tx *sql.Tx, situationID string, version int, tenantID string) (*snapshotEvidence, error) {
	snapshotJSON, persistedDigest, traceparent, tracestate, err := a.loadSnapshotJSON(ctx, tx, situationID, version)
	if err != nil {
		return nil, fmt.Errorf("load snapshot: %w", err)
	}
	var snapshot map[string]any
	if err := json.Unmarshal(snapshotJSON, &snapshot); err != nil {
		return nil, fmt.Errorf("unmarshal snapshot: %w", err)
	}
	if err := contractsv1.Validate(contractsv1.SchemaSnapshot, snapshot); err != nil {
		return nil, fmt.Errorf("validate snapshot: %w", err)
	}
	if snapshotString(snapshot, "situation_id") != situationID ||
		snapshotInt(snapshot, "situation_version") != version ||
		snapshotString(snapshot, "tenant_id") != tenantID {
		return nil, fmt.Errorf("snapshot identity does not match episode admission")
	}
	entityID, err := snapshotEntityID(snapshotJSON)
	if err != nil {
		return nil, fmt.Errorf("load snapshot entity: %w", err)
	}
	digest, err := canonicaljson.Digest(canonicaljson.DomainSnapshot, snapshot)
	if err != nil {
		return nil, fmt.Errorf("digest snapshot: %w", err)
	}
	decodedDigest, err := canonicaljson.DecodeDigest(digest)
	if err != nil || !bytes.Equal(decodedDigest, persistedDigest) {
		return nil, fmt.Errorf("snapshot digest does not match persisted situation version")
	}
	return &snapshotEvidence{
		json: snapshotJSON, document: snapshot, entityID: entityID, digest: digest,
		traceparent: traceparent, tracestate: tracestate,
	}, nil
}

type schedulerItem struct {
	SchedulerItemID  string
	Kind             string
	TriggerID        string
	TenantID         string
	SituationID      string
	SituationVersion int
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

type evaluation struct {
	TriggerID   string
	TriggerName string
	Score       float64
	Threshold   float64
	Lane        string
	DeltaJSON   []byte
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

func snapshotString(snapshot map[string]any, key string) string {
	value, _ := snapshot[key].(string)
	return value
}

func snapshotInt(snapshot map[string]any, key string) int {
	value, _ := snapshot[key].(float64)
	return int(value)
}
