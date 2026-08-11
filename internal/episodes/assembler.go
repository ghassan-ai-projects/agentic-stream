// Package episodes assembles deterministic episode requests from scheduler
// items and persists episode records for execution.
package episodes

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/canonicaljson"
	"github.com/ghassan-ai-projects/agentic-stream/internal/ids"
	"github.com/ghassan-ai-projects/agentic-stream/internal/spec"
)

// Request is the durable input to an episode executor. Each field maps to a
// column in the episodes table; RequestJSON is the canonical executor input.
type Request struct {
	EpisodeID        string // unique episode identity.
	SchedulerItemID  string // scheduler item that admitted this episode.
	TenantID         string // tenant owning the situation.
	SituationID      string // situation being reasoned about.
	SituationVersion int    // immutable situation version bound to this episode.
	ExecutorName     string // executor configured in the active spec.
	ExecutorVersion  string // digest of the active spec.
	ModelPolicy      string // model policy from the spec executor.
	PromptVersion    string // prompt version from the spec executor.
	SnapshotSHA256   string // deterministic hash of the snapshot subset.
	AdmissionKey     []byte // unique 32-byte admission key.
	RequestJSON      []byte // canonical JSON sent to the executor.
}

// Assembler builds deterministic episode requests.
type Assembler struct {
	spec  *spec.CompiledSpec
	idGen ids.Generator
}

// NewAssembler creates an assembler for the given spec.
func NewAssembler(compiled *spec.CompiledSpec, idGen ids.Generator) *Assembler {
	if idGen == nil {
		idGen = ids.Random()
	}
	return &Assembler{spec: compiled, idGen: idGen}
}

// Assemble builds a Request from a pending scheduler item. It loads the trigger
// evaluation and situation version inside the supplied transaction and returns
// a ready-to-persist Request without mutating the database.
func (a *Assembler) Assemble(ctx context.Context, tx *sql.Tx, schedulerItemID, tenantID string) (*Request, error) {
	item, err := a.loadSchedulerItem(ctx, tx, schedulerItemID)
	if err != nil {
		return nil, fmt.Errorf("load scheduler item: %w", err)
	}
	if item.TenantID != tenantID {
		return nil, fmt.Errorf("tenant mismatch: item belongs to %s, requested %s", item.TenantID, tenantID)
	}

	ev, err := a.loadEvaluation(ctx, tx, item.TriggerID)
	if err != nil {
		return nil, fmt.Errorf("load evaluation: %w", err)
	}

	snapshotJSON, err := a.loadSnapshotJSON(ctx, tx, item.SituationID, item.SituationVersion)
	if err != nil {
		return nil, fmt.Errorf("load snapshot: %w", err)
	}

	var snapshot map[string]any
	if err := json.Unmarshal(snapshotJSON, &snapshot); err != nil {
		return nil, fmt.Errorf("unmarshal snapshot: %w", err)
	}

	var delta map[string]any
	if len(ev.DeltaJSON) > 0 {
		if err := json.Unmarshal(ev.DeltaJSON, &delta); err != nil {
			return nil, fmt.Errorf("unmarshal delta: %w", err)
		}
	}

	tools := a.buildTools()
	request := map[string]any{
		"episode_id":        a.idGen.New(ids.PrefixEpisode),
		"scheduler_item_id": schedulerItemID,
		"tenant_id":         tenantID,
		"situation_id":      item.SituationID,
		"situation_version": item.SituationVersion,
		"trigger": map[string]any{
			"trigger_id":   ev.TriggerID,
			"trigger_name": ev.TriggerName,
			"score":        ev.Score,
			"threshold":    ev.Threshold,
			"lane":         ev.Lane,
		},
		"snapshot": snapshot,
		"delta":    delta,
		"tools":    tools,
		"executor": map[string]any{
			"name":            a.spec.Cognition.Executor.Name,
			"model_policy":    a.spec.Cognition.Executor.ModelPolicy,
			"prompt_version":  a.spec.Cognition.Executor.PromptVersion,
			"objective":       a.spec.Cognition.Executor.Objective,
			"decision_schema": a.spec.Cognition.Executor.DecisionSchema,
		},
		"budget": a.budgetMap(),
	}

	episodeID := request["episode_id"].(string)
	admissionKey := sha256.Sum256([]byte(episodeID + "|" + schedulerItemID))

	// The deterministic snapshot projection excludes the generated episode
	// identity so that the same situation/trigger always yields the same digest.
	snapshotOnly := map[string]any{
		"situation_id":      item.SituationID,
		"situation_version": item.SituationVersion,
		"trigger":           request["trigger"],
		"snapshot":          snapshot,
		"delta":             delta,
		"tools":             tools,
		"executor":          request["executor"],
		"budget":            request["budget"],
	}
	snapshotDigest, err := canonicaljson.Digest(canonicaljson.DomainSnapshot, snapshotOnly)
	if err != nil {
		return nil, fmt.Errorf("digest snapshot: %w", err)
	}
	request["snapshot_digest"] = snapshotDigest
	requestJSON, err := canonicaljson.Marshal(request)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	return &Request{
		EpisodeID:        episodeID,
		SchedulerItemID:  schedulerItemID,
		TenantID:         tenantID,
		SituationID:      item.SituationID,
		SituationVersion: item.SituationVersion,
		ExecutorName:     a.spec.Cognition.Executor.Name,
		ExecutorVersion:  a.spec.Digest,
		ModelPolicy:      a.spec.Cognition.Executor.ModelPolicy,
		PromptVersion:    a.spec.Cognition.Executor.PromptVersion,
		SnapshotSHA256:   snapshotDigest,
		AdmissionKey:     admissionKey[:],
		RequestJSON:      requestJSON,
	}, nil
}

// Persist saves the episode request to the episodes table and marks the
// scheduler item as admitted. It runs inside the supplied transaction. The
// scheduler item must still be pending; otherwise Persist returns an error.
func (a *Assembler) Persist(ctx context.Context, tx *sql.Tx, req *Request, now time.Time) error {
	snapshotHash, err := canonicaljson.DecodeDigest(req.SnapshotSHA256)
	if err != nil {
		return fmt.Errorf("decode snapshot digest: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO episodes (
			episode_id, scheduler_item_id, tenant_id, situation_id, situation_version,
			executor_name, executor_version, model_policy, prompt_version,
			snapshot_sha256, admission_key, request_json, lifecycle_status, accepted_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 'admitted', ?)`,
		req.EpisodeID, req.SchedulerItemID, req.TenantID, req.SituationID, req.SituationVersion,
		req.ExecutorName, req.ExecutorVersion, req.ModelPolicy, req.PromptVersion,
		snapshotHash, req.AdmissionKey, req.RequestJSON,
		now.Format(time.RFC3339Nano),
	); err != nil {
		return fmt.Errorf("insert episode: %w", err)
	}
	res, err := tx.ExecContext(ctx, `
		UPDATE scheduler_items SET status = 'admitted', updated_at = ?
		WHERE scheduler_item_id = ? AND status = 'pending'`,
		now.Format(time.RFC3339Nano), req.SchedulerItemID,
	)
	if err != nil {
		return fmt.Errorf("mark scheduler item admitted: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("rows affected: %w", err)
	}
	if n == 0 {
		return fmt.Errorf("scheduler item %s is no longer pending", req.SchedulerItemID)
	}
	return nil
}

func (a *Assembler) buildTools() []map[string]any {
	tools := make([]map[string]any, 0, len(a.spec.Actions.Intents))
	for _, intent := range a.spec.Actions.Intents {
		tools = append(tools, map[string]any{
			"type":        intent.Type,
			"risk":        intent.Risk,
			"schema":      intent.Schema,
			"policy":      intent.Policy,
			"rate_limit":  intent.RateLimitPerHour,
			"allowed":     true,
			"description": "",
		})
	}
	return tools
}

func (a *Assembler) budgetMap() map[string]any {
	b := a.spec.Cognition.Executor.Budget
	return map[string]any{
		"wall_time":               b.WallTime,
		"model_calls":             b.ModelCalls,
		"input_tokens":            b.InputTokens,
		"output_tokens":           b.OutputTokens,
		"tool_calls":              b.ToolCalls,
		"tool_result_bytes":       b.ToolResultBytes,
		"total_tool_result_bytes": b.TotalToolResultBytes,
		"provider_retries":        b.ProviderRetries,
		"cost_microunits":         b.CostMicrounits,
	}
}

type schedulerItem struct {
	SchedulerItemID  string
	TriggerID        string
	TenantID         string
	SituationID      string
	SituationVersion int
}

func (a *Assembler) loadSchedulerItem(ctx context.Context, tx *sql.Tx, id string) (schedulerItem, error) {
	var item schedulerItem
	if err := tx.QueryRowContext(ctx, `
		SELECT scheduler_item_id, trigger_id, tenant_id, situation_id, situation_version
		FROM scheduler_items WHERE scheduler_item_id = ?`,
		id,
	).Scan(&item.SchedulerItemID, &item.TriggerID, &item.TenantID, &item.SituationID, &item.SituationVersion); err != nil {
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

func (a *Assembler) loadSnapshotJSON(ctx context.Context, tx *sql.Tx, situationID string, version int) ([]byte, error) {
	var snapshotJSON []byte
	if err := tx.QueryRowContext(ctx, `
		SELECT snapshot_json FROM situation_versions
		WHERE situation_id = ? AND version = ?`,
		situationID, version,
	).Scan(&snapshotJSON); err != nil {
		return nil, fmt.Errorf("query situation version: %w", err)
	}
	return snapshotJSON, nil
}
