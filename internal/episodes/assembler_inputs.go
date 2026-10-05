package episodes

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"

	"github.com/ghassan-ai-projects/agentic-stream/internal/contractsv1"
	domain "github.com/ghassan-ai-projects/agentic-stream/internal/episodes/internal/domain"
	store "github.com/ghassan-ai-projects/agentic-stream/internal/episodes/internal/store"
)

// assemblyInputs are the durable facts one scheduler item is assembled from.
type assemblyInputs struct {
	evaluation      evaluation
	snapshot        *domain.SnapshotEvidence
	delta           map[string]any
	reconsideration map[string]any
}

type schedulerItem = store.SchedulerItem

type evaluation = store.Evaluation

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
	return store.LoadEvaluation(ctx, tx, triggerID) //nolint:wrapcheck // Store owns the query error context.
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
	snapshotJSON, snapshotDigest, traceparent, tracestate, err := store.LoadSnapshot(ctx, tx, situationID, version)
	if err != nil {
		return nil, nil, "", "", err
	}
	if _, err := contractsv1.ParseTraceContext(traceparent, tracestate); err != nil {
		return nil, nil, "", "", fmt.Errorf("validate situation trace context: %w", err)
	}
	return snapshotJSON, snapshotDigest, traceparent, tracestate, nil
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
	return store.LoadSchedulerItem(ctx, tx, id) //nolint:wrapcheck // Store owns the query error context.
}
