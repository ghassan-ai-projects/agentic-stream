package episodes

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/clock"
	"github.com/ghassan-ai-projects/agentic-stream/internal/ids"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
)

// Executor runs a bounded episode against an Episode Request and returns a
// terminal outcome. Implementations must not mutate stream or action state
// directly; all effects are returned as typed Decisions and Intents.
type Executor interface {
	// Execute runs the episode to completion or budget exhaustion.
	Execute(ctx context.Context, req *Request) (*Outcome, error)
	// Name returns the executor identifier recorded in the episode ledger.
	Name() string
}

// Outcome is the terminal result of one worker attempt.
type Outcome struct {
	Status         string   `json:"status"`
	DecisionJSON   []byte   `json:"decision_json,omitempty"`
	DecisionSHA256 string   `json:"decision_sha256,omitempty"`
	Reasons        []string `json:"reasons,omitempty"`
}

// Runner polls admitted episodes and executes them deterministically.
type Runner struct {
	db       *storage.DB
	executor Executor
	clk      clock.Clock
	idGen    ids.Generator
}

// NewRunner creates a runner for the given executor and clock.
func NewRunner(db *storage.DB, executor Executor, clk clock.Clock, idGen ids.Generator) *Runner {
	if clk == nil {
		clk = clock.Physical()
	}
	if idGen == nil {
		idGen = ids.Random()
	}
	return &Runner{db: db, executor: executor, clk: clk, idGen: idGen}
}

// RunOnce finds one admitted episode, fences a worker attempt, executes it,
// and persists the attempt terminal state and proposed Decision.
// It returns true if an episode was processed.
func (r *Runner) RunOnce(ctx context.Context, tenantID string) (bool, error) {
	var req Request
	var episodeID string
	var identity Identity
	err := r.withTx(ctx, func(tx *sql.Tx) error {
		if err := tx.QueryRowContext(ctx, `
			SELECT episode_id, scheduler_item_id, tenant_id, situation_id, situation_version,
			       executor_name, executor_version, model_policy, prompt_version,
			       snapshot_sha256, admission_key, request_json
			FROM episodes
			WHERE tenant_id = ? AND lifecycle_status = 'admitted'
			ORDER BY accepted_at LIMIT 1`,
			tenantID,
		).Scan(
			&episodeID, &req.SchedulerItemID, &req.TenantID, &req.SituationID, &req.SituationVersion,
			&req.ExecutorName, &req.ExecutorVersion, &req.ModelPolicy, &req.PromptVersion,
			&req.SnapshotSHA256, &req.AdmissionKey, &req.RequestJSON,
		); err != nil {
			if err == sql.ErrNoRows {
				return nil
			}
			return fmt.Errorf("query admitted episode: %w", err)
		}

		req.EpisodeID = episodeID
		attemptID := r.idGen.New(ids.PrefixAttempt)
		var err error
		identity, err = StartAttempt(ctx, tx, episodeID, attemptID, r.clk.Now())
		if err != nil {
			return fmt.Errorf("start episode attempt: %w", err)
		}
		if err := TransitionAttempt(ctx, tx, identity, AttemptRunning, r.clk.Now(), nil); err != nil {
			return fmt.Errorf("mark episode attempt running: %w", err)
		}
		return nil
	})
	if err != nil {
		return false, err
	}
	if episodeID == "" {
		return false, nil
	}

	outcome, err := r.executor.Execute(ctx, &req)
	if err != nil {
		return true, r.failAttempt(ctx, identity, fmt.Errorf("execute: %w", err).Error())
	}

	return true, r.withTx(ctx, func(tx *sql.Tx) error {
		now := r.clk.Now().UTC().Format(time.RFC3339Nano)
		if outcome.DecisionJSON != nil {
			decisionID := r.idGen.New(ids.PrefixDecision)
			h := sha256.Sum256(outcome.DecisionJSON)
			if _, err := tx.ExecContext(ctx, `
				INSERT INTO decisions (
					decision_id, episode_id, attempt_id, fence, ordinal, situation_id,
					situation_version, raw_json, decision_sha256, validation_status,
					validation_json, created_at
				) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, 'proposed', ?, ?)`,
				decisionID, episodeID, identity.AttemptID, identity.Fence, 1,
				req.SituationID, req.SituationVersion,
				outcome.DecisionJSON, h[:], []byte(`{}`), now,
			); err != nil {
				return fmt.Errorf("insert decision: %w", err)
			}
		}

		attemptStatus := AttemptStatus(outcome.Status)
		if outcome.DecisionJSON != nil {
			attemptStatus = AttemptProduced
		} else if attemptStatus == "" {
			attemptStatus = AttemptDeclined
		}
		if !IsTerminalAttempt(attemptStatus) {
			return fmt.Errorf("executor returned non-terminal attempt status %q", attemptStatus)
		}
		terminalJSON, err := json.Marshal(outcome)
		if err != nil {
			return fmt.Errorf("marshal outcome: %w", err)
		}
		if err := TransitionAttempt(ctx, tx, identity, attemptStatus, r.clk.Now(), terminalJSON); err != nil {
			return fmt.Errorf("finish episode attempt: %w", err)
		}
		if _, err := tx.ExecContext(ctx, `
			UPDATE episodes SET lifecycle_status = 'concluded', ended_at = ?, terminal_json = ?
			WHERE episode_id = ?`,
			now, terminalJSON, episodeID,
		); err != nil {
			return fmt.Errorf("update episode terminal: %w", err)
		}
		return nil
	})
}

func (r *Runner) failAttempt(ctx context.Context, identity Identity, reason string) error {
	return r.withTx(ctx, func(tx *sql.Tx) error {
		terminalJSON, err := json.Marshal(map[string]any{"status": AttemptFailed, "reason": reason})
		if err != nil {
			return fmt.Errorf("marshal terminal: %w", err)
		}
		if err := TransitionAttempt(ctx, tx, identity, AttemptFailed, r.clk.Now(), terminalJSON); err != nil {
			return fmt.Errorf("finish failed episode attempt: %w", err)
		}
		if _, err := tx.ExecContext(ctx, `
			UPDATE episodes SET lifecycle_status = 'concluded', ended_at = ?, terminal_json = ?
			WHERE episode_id = ?`,
			r.clk.Now().UTC().Format(time.RFC3339Nano), terminalJSON, identity.EpisodeID,
		); err != nil {
			return fmt.Errorf("update episode failed: %w", err)
		}
		return nil
	})
}

func (r *Runner) withTx(ctx context.Context, fn func(*sql.Tx) error) error {
	if err := r.db.WithTx(ctx, fn); err != nil {
		return fmt.Errorf("tx: %w", err)
	}
	return nil
}
