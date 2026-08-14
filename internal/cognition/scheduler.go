package cognition

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"errors"
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

func (s *Scheduler) findTrigger(name string) (spec.Trigger, error) {
	for _, tr := range s.spec.Cognition.Triggers {
		if tr.Name == name {
			return tr, nil
		}
	}
	return spec.Trigger{}, fmt.Errorf("trigger %q not found", name)
}

func (s *Scheduler) latestAdmittedTime(ctx context.Context, tx *sql.Tx, situationID, triggerName, excludeTriggerID string) (*time.Time, error) {
	var evaluatedAt string
	if err := tx.QueryRowContext(ctx, `
		SELECT evaluated_at FROM trigger_evaluations
		WHERE situation_id = ? AND trigger_name = ? AND outcome = 'admitted' AND trigger_id != ?
		ORDER BY evaluated_at DESC LIMIT 1`,
		situationID, triggerName, excludeTriggerID,
	).Scan(&evaluatedAt); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("query latest admitted: %w", err)
	}
	t, err := time.Parse(time.RFC3339Nano, evaluatedAt)
	if err != nil {
		return nil, fmt.Errorf("parse evaluated_at: %w", err)
	}
	return &t, nil
}

func (s *Scheduler) countPending(ctx context.Context, tx *sql.Tx, tenantID string) (int, error) {
	var count int
	if err := tx.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM scheduler_items WHERE tenant_id = ? AND status = 'pending'",
		tenantID,
	).Scan(&count); err != nil {
		return 0, fmt.Errorf("query pending count: %w", err)
	}
	return count, nil
}

func (s *Scheduler) countPendingSameTrigger(ctx context.Context, tx *sql.Tx, situationID, triggerName string) (int, error) {
	var count int
	if err := tx.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM scheduler_items si
		JOIN trigger_evaluations te ON te.trigger_id = si.trigger_id
		WHERE si.situation_id = ? AND te.trigger_name = ? AND si.status = 'pending'`,
		situationID, triggerName,
	).Scan(&count); err != nil {
		return 0, fmt.Errorf("query pending same-trigger count: %w", err)
	}
	return count, nil
}

func (s *Scheduler) supersedePending(ctx context.Context, tx *sql.Tx, situationID, triggerName string) error {
	now := s.clk.Now().UTC().Format(time.RFC3339Nano)
	var tenantID string
	var replacementVersion int
	if err := tx.QueryRowContext(ctx, "SELECT tenant_id, current_version FROM situations WHERE situation_id = ?", situationID).Scan(&tenantID, &replacementVersion); err != nil {
		return fmt.Errorf("load supersession situation: %w", err)
	}
	var traceparent, tracestate sql.NullString
	if err := tx.QueryRowContext(ctx, "SELECT traceparent, tracestate FROM situation_versions WHERE situation_id = ? AND version = ?", situationID, replacementVersion).Scan(&traceparent, &tracestate); err != nil && !errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("load supersession trace: %w", err)
	}
	type supersededItem struct {
		id      string
		version int
	}
	rows, err := tx.QueryContext(ctx, `
		SELECT scheduler_item_id, situation_version
		FROM scheduler_items
		WHERE situation_id = ? AND trigger_id IN (
			SELECT trigger_id FROM trigger_evaluations
			WHERE situation_id = ? AND trigger_name = ? AND outcome = 'admitted'
		) AND status IN ('pending', 'admitted')
		ORDER BY scheduler_item_id`, situationID, situationID, triggerName)
	if err != nil {
		return fmt.Errorf("find superseded scheduler items: %w", err)
	}
	items := make([]supersededItem, 0)
	for rows.Next() {
		var item supersededItem
		if err := rows.Scan(&item.id, &item.version); err != nil {
			_ = rows.Close()
			return fmt.Errorf("scan superseded scheduler item: %w", err)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return fmt.Errorf("iterate superseded scheduler items: %w", err)
	}
	if err := rows.Close(); err != nil {
		return fmt.Errorf("close superseded scheduler items: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE scheduler_items SET status = 'coalesced', updated_at = ?
		WHERE situation_id = ? AND trigger_id IN (
			SELECT trigger_id FROM trigger_evaluations
			WHERE situation_id = ? AND trigger_name = ? AND outcome = 'admitted'
		) AND status IN ('pending', 'admitted')`,
		now, situationID, situationID, triggerName,
	); err != nil {
		return fmt.Errorf("supersede scheduler items: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE episodes SET lifecycle_status = 'superseded', ended_at = ?
		WHERE scheduler_item_id IN (
			SELECT scheduler_item_id FROM scheduler_items
			WHERE situation_id = ? AND status = 'coalesced'
		) AND lifecycle_status IN ('admitted', 'running')`,
		now, situationID,
	); err != nil {
		return fmt.Errorf("supersede episodes: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE episode_attempts SET status = 'cancelling'
		WHERE episode_id IN (
			SELECT episode_id FROM episodes
			WHERE scheduler_item_id IN (
				SELECT scheduler_item_id FROM scheduler_items
				WHERE situation_id = ? AND status = 'coalesced'
			) AND lifecycle_status = 'superseded'
		) AND status IN ('dispatched', 'running')`,
		situationID,
	); err != nil {
		return fmt.Errorf("cancel superseded attempts: %w", err)
	}
	for _, item := range items {
		if item.version >= replacementVersion {
			continue
		}
		if err := notify.AppendLifecycleEventWithTrace(ctx, tx,
			"situation.superseded:"+situationID+":"+fmt.Sprint(item.version)+":"+fmt.Sprint(replacementVersion)+":"+item.id,
			tenantID, notify.TypeSituationSuperseded, "situation/"+situationID, situationID,
			map[string]any{
				"tenant_id": tenantID, "situation_id": situationID,
				"superseded_version": item.version, "replacement_version": replacementVersion,
				"reason": "newer_situation_version_admitted", "source_authority": notify.SourceForTenant(tenantID),
			}, s.clk.Now().UTC(), contractsv1.TraceContext{Traceparent: traceparent.String, Tracestate: tracestate.String}); err != nil {
			return fmt.Errorf("append situation superseded notification: %w", err)
		}
	}
	if err := s.withdrawSupersededApprovals(ctx, tx, situationID, tenantID, replacementVersion, now); err != nil {
		return err
	}
	return nil
}

func (s *Scheduler) withdrawSupersededApprovals(ctx context.Context, tx *sql.Tx, situationID, tenantID string, replacementVersion int, now string) error {
	rows, err := tx.QueryContext(ctx, `
		SELECT a.approval_id, i.intent_id, i.situation_id, i.situation_version,
		       d.traceparent, d.tracestate
		FROM approvals a
		JOIN intents i ON i.intent_id = a.intent_id
		JOIN decisions d ON d.decision_id = i.decision_id
		WHERE i.situation_id = ? AND i.situation_version < ? AND a.status = 'pending'
		ORDER BY a.approval_id`, situationID, replacementVersion)
	if err != nil {
		return fmt.Errorf("find superseded approvals: %w", err)
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var approvalID, intentID, linkedSituationID string
		var situationVersion int
		var traceparent, tracestate sql.NullString
		if err := rows.Scan(&approvalID, &intentID, &linkedSituationID, &situationVersion, &traceparent, &tracestate); err != nil {
			return fmt.Errorf("scan superseded approval: %w", err)
		}
		if _, err := tx.ExecContext(ctx, `
			UPDATE approvals SET status = 'denied', decided_at = ?, withdrawn_at = ?, withdrawal_reason = ?, reason = ?
			WHERE approval_id = ? AND status = 'pending'`, now, now, "situation_version_conflict", "approval_withdrawn", approvalID); err != nil {
			return fmt.Errorf("withdraw superseded approval %s: %w", approvalID, err)
		}
		if err := notify.AppendLifecycleEventWithTrace(ctx, tx, "approval.withdrawn:"+approvalID, tenantID, notify.TypeApprovalWithdrawn, "approval/"+approvalID, linkedSituationID, map[string]any{
			"tenant_id": tenantID, "approval_id": approvalID, "intent_id": intentID, "situation_id": linkedSituationID,
			"situation_version": situationVersion, "reason": "situation_version_conflict", "source_authority": notify.SourceForTenant(tenantID),
		}, s.clk.Now().UTC(), contractsv1.TraceContext{Traceparent: traceparent.String, Tracestate: tracestate.String}); err != nil {
			return fmt.Errorf("append superseded approval notification: %w", err)
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate superseded approvals: %w", err)
	}
	return nil
}

func (s *Scheduler) insertItem(ctx context.Context, tx *sql.Tx, item Item, tenantID string) error {
	notBefore := sql.NullString{}
	if item.NotBefore != nil {
		notBefore = sql.NullString{String: item.NotBefore.Format(time.RFC3339Nano), Valid: true}
	}
	now := s.clk.Now().UTC().Format(time.RFC3339Nano)
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO scheduler_items (
			scheduler_item_id, trigger_id, tenant_id, situation_id, situation_version,
			kind, lane, priority, status, dedupe_key, not_before, expires_at, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(trigger_id) DO UPDATE SET
			kind = excluded.kind,
			situation_version = excluded.situation_version,
			lane = excluded.lane,
			priority = excluded.priority,
			status = excluded.status,
			dedupe_key = excluded.dedupe_key,
			not_before = excluded.not_before,
			expires_at = excluded.expires_at,
			updated_at = excluded.updated_at`,
		item.SchedulerItemID, item.TriggerID, tenantID, item.SituationID, item.SituationVersion, item.Kind,
		item.Lane, item.Priority, item.Status, s.dedupeKey(item.SituationID, item.SituationVersion, item.TriggerID),
		notBefore, item.ExpiresAt.Format(time.RFC3339Nano), now, now,
	); err != nil {
		return fmt.Errorf("upsert scheduler item: %w", err)
	}
	return nil
}

func (s *Scheduler) dedupeKey(situationID string, version int, triggerID string) []byte {
	h := sha256.New()
	_, _ = fmt.Fprintf(h, "%s|%d|%s", situationID, version, triggerID)
	return h.Sum(nil)
}

func (s *Scheduler) itemID() string {
	return s.idGen.New(ids.PrefixScheduler)
}
