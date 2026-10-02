package cognition

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/canonicaljson"
	"github.com/ghassan-ai-projects/agentic-stream/internal/clock"
	"github.com/ghassan-ai-projects/agentic-stream/internal/contractsv1"
	"github.com/ghassan-ai-projects/agentic-stream/internal/duration"
	"github.com/ghassan-ai-projects/agentic-stream/internal/ids"
	"github.com/ghassan-ai-projects/agentic-stream/internal/notify"
	"github.com/ghassan-ai-projects/agentic-stream/internal/situations"
	"github.com/ghassan-ai-projects/agentic-stream/internal/spec"
)

// Scheduler manages the durable cognition queue.
type Scheduler struct {
	spec  *spec.CompiledSpec
	idGen ids.Generator
	clk   clock.Clock
}

// NewScheduler creates a scheduler configured by spec.
func NewScheduler(compiled *spec.CompiledSpec, idGen ids.Generator, clk clock.Clock) *Scheduler {
	if clk == nil {
		clk = clock.Physical()
	}
	if idGen == nil {
		idGen = ids.Random()
	}
	return &Scheduler{spec: compiled, idGen: idGen, clk: clk}
}

// Item is one durable scheduler entry.
type Item struct {
	SchedulerItemID  string     // unique scheduler item identity.
	Kind             string     // standard or reconsider.
	TriggerID        string     // trigger evaluation that admitted this item.
	SituationID      string     // situation being reasoned about.
	SituationVersion int        // immutable situation version bound to this item.
	Lane             string     // fast or deep lane.
	Priority         float64    // admission score used for ordering.
	Status           string     // pending, admitted, coalesced, expired, canceled, completed.
	NotBefore        *time.Time // earliest time the item may be picked.
	ExpiresAt        time.Time  // latest time the item remains useful.
}

const (
	defaultExpiresAfter = 15 * time.Minute
	globalCapacity      = 100
)

func parseOptionalDuration(s string, defaultDur time.Duration) (time.Duration, error) {
	if s == "" {
		return defaultDur, nil
	}
	d, err := duration.Parse(s)
	if err != nil {
		return 0, fmt.Errorf("parse duration %q: %w", s, err)
	}
	return d, nil
}

// Admit persists the trigger evaluation and creates or updates the durable
// scheduler item. It runs inside the supplied transaction.
func (s *Scheduler) Admit(ctx context.Context, tx *sql.Tx, eval Evaluation, v situations.Version, tenantID, deploymentID string) error {
	if err := s.saveEvaluation(ctx, tx, eval, tenantID, deploymentID); err != nil {
		return fmt.Errorf("save evaluation: %w", err)
	}

	// Ignored or rejected evaluations do not create queue items.
	if eval.Outcome != "admitted" {
		return nil
	}

	item, err := s.buildItem(ctx, tx, eval)
	if err != nil {
		return fmt.Errorf("build item: %w", err)
	}

	pending, err := s.countPending(ctx, tx, tenantID)
	if err != nil {
		return fmt.Errorf("count pending: %w", err)
	}
	staleSameTrigger, err := s.countPendingSameTrigger(ctx, tx, eval.SituationID, eval.TriggerName)
	if err != nil {
		return fmt.Errorf("count stale same-trigger: %w", err)
	}

	// If global capacity is exhausted, we can still admit this version if it
	// replaces a stale pending item for the same situation and trigger.
	if pending >= globalCapacity && staleSameTrigger == 0 {
		eval.Outcome = "deferred"
		eval.Reasons = append(eval.Reasons, "global capacity exhausted")
		if err := s.saveEvaluation(ctx, tx, eval, tenantID, deploymentID); err != nil {
			return fmt.Errorf("save deferred evaluation: %w", err)
		}
		return nil
	}

	// Supersede stale pending items for the same situation and trigger. This
	// also marks any already-created episodes as superseded so the new episode
	// can be inserted under the one-live-episode-per-situation constraint.
	if err := s.supersedePending(ctx, tx, eval.SituationID, eval.TriggerName); err != nil {
		return fmt.Errorf("supersede pending: %w", err)
	}

	if err := s.insertItem(ctx, tx, item, tenantID); err != nil {
		return fmt.Errorf("insert item: %w", err)
	}

	return nil
}

func (s *Scheduler) saveEvaluation(ctx context.Context, tx *sql.Tx, eval Evaluation, tenantID, deploymentID string) error {
	reasonsJSON, err := json.Marshal(eval.Reasons)
	if err != nil {
		return fmt.Errorf("marshal reasons: %w", err)
	}
	policySHA, err := canonicaljson.DecodeDigest(s.spec.Digest)
	if err != nil {
		return fmt.Errorf("decode policy digest: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO trigger_evaluations (
			trigger_id, tenant_id, deployment_id, trigger_name,
			situation_id, situation_version, score, threshold, lane,
			outcome, reasons_json, policy_sha256, delta_json, evaluated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(trigger_id) DO UPDATE SET
			situation_version = excluded.situation_version,
			score = excluded.score,
			threshold = excluded.threshold,
			lane = excluded.lane,
			outcome = excluded.outcome,
			reasons_json = excluded.reasons_json,
			policy_sha256 = excluded.policy_sha256,
			delta_json = excluded.delta_json,
			evaluated_at = excluded.evaluated_at`,
		eval.TriggerID, tenantID, deploymentID, eval.TriggerName,
		eval.SituationID, eval.SituationVersion, eval.Score, eval.Threshold, eval.Lane,
		eval.Outcome, reasonsJSON, policySHA[:], eval.DeltaJSON, eval.EvaluatedAt.Format(time.RFC3339Nano),
	); err != nil {
		return fmt.Errorf("upsert trigger evaluation: %w", err)
	}
	event := contractsv1.CloudEvent{
		SpecVersion: "1.0", ID: eval.TriggerID + ":" + eval.Outcome + ":" + eval.EvaluatedAt.UTC().Format(time.RFC3339Nano), Source: "//agentic-stream/tenants/" + tenantID,
		Type: "situation.trigger.evaluated", Subject: "situation/" + eval.SituationID,
		Time: eval.EvaluatedAt, DataContentType: "application/json",
		DataSchema: "urn:situation-runtime:schema:trigger-evaluation:v1",
		Data:       map[string]any{"trigger_id": eval.TriggerID, "situation_id": eval.SituationID, "situation_version": eval.SituationVersion, "outcome": eval.Outcome},
		TenantID:   tenantID, PartitionKey: eval.SituationID, IngestedTime: eval.EvaluatedAt,
		Classification: contractsv1.ClassificationInternal,
	}
	event.EnvelopeDigest, err = event.ComputeEnvelopeDigest()
	if err != nil {
		return fmt.Errorf("digest trigger notification: %w", err)
	}
	if _, err := notify.Append(ctx, tx, event, eval.EvaluatedAt); err != nil {
		return fmt.Errorf("append trigger notification: %w", err)
	}
	return nil
}

func (s *Scheduler) buildItem(ctx context.Context, tx *sql.Tx, eval Evaluation) (Item, error) {
	item := Item{
		SchedulerItemID:  s.itemID(),
		Kind:             "standard",
		TriggerID:        eval.TriggerID,
		SituationID:      eval.SituationID,
		SituationVersion: eval.SituationVersion,
		Lane:             eval.Lane,
		Priority:         eval.Score,
		Status:           "pending",
	}

	trigger, err := s.findTrigger(eval.TriggerName)
	if err != nil {
		return item, err
	}

	now := s.clk.Now().UTC()
	expiresAfter, err := parseOptionalDuration(trigger.ExpiresAfter, defaultExpiresAfter)
	if err != nil {
		return item, fmt.Errorf("parse expiresAfter: %w", err)
	}
	item.ExpiresAt = now.Add(expiresAfter)

	debounce, err := parseOptionalDuration(trigger.Debounce, 0)
	if err != nil {
		return item, fmt.Errorf("parse debounce: %w", err)
	}
	if debounce > 0 {
		notBefore := now.Add(debounce)
		item.NotBefore = &notBefore
	}

	cooldown, err := parseOptionalDuration(trigger.Cooldown, 0)
	if err != nil {
		return item, fmt.Errorf("parse cooldown: %w", err)
	}
	if cooldown > 0 {
		latest, err := s.latestAdmittedTime(ctx, tx, eval.SituationID, eval.TriggerName, eval.TriggerID)
		if err != nil {
			return item, err
		}
		if latest != nil {
			notBefore := latest.Add(cooldown)
			if item.NotBefore == nil || notBefore.After(*item.NotBefore) {
				item.NotBefore = &notBefore
			}
		}
	}

	return item, nil
}
