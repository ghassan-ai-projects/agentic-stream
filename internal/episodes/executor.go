package episodes

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/canonicaljson"
	"github.com/ghassan-ai-projects/agentic-stream/internal/clock"
	"github.com/ghassan-ai-projects/agentic-stream/internal/contractsv1"
	"github.com/ghassan-ai-projects/agentic-stream/internal/costcontrol"
	"github.com/ghassan-ai-projects/agentic-stream/internal/decisions"
	"github.com/ghassan-ai-projects/agentic-stream/internal/ids"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
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
	AttemptID      string   `json:"attempt_id,omitempty"`
	Fence          int64    `json:"fence,omitempty"`
	DecisionJSON   []byte   `json:"decision_json,omitempty"`
	DecisionSHA256 string   `json:"decision_sha256,omitempty"`
	Reasons        []string `json:"reasons,omitempty"`
	CostMicrounits uint64   `json:"cost_microunits,omitempty"`
}

// Runner polls admitted episodes and executes them deterministically.
type Runner struct {
	db         *storage.DB
	executor   Executor
	clk        clock.Clock
	idGen      ids.Generator
	ownerEpoch string
	cost       *costcontrol.Controller
}

const maxEpisodeAttempts = 3

// WithCostControl enables settlement of durable episode cost reservations.
func (r *Runner) WithCostControl(controller *costcontrol.Controller) *Runner {
	r.cost = controller
	return r
}

// NewRunner creates a runner for the given executor and clock.
func NewRunner(db *storage.DB, executor Executor, clk clock.Clock, idGen ids.Generator) *Runner {
	return NewRunnerWithEpoch(db, executor, clk, idGen, "")
}

// NewRunnerWithEpoch creates a runner that fences every attempt to ownerEpoch.
// Live runtime composition must use a freshly claimed epoch.
func NewRunnerWithEpoch(db *storage.DB, executor Executor, clk clock.Clock, idGen ids.Generator, ownerEpoch string) *Runner {
	if clk == nil {
		clk = clock.Physical()
	}
	if idGen == nil {
		idGen = ids.Random()
	}
	return &Runner{db: db, executor: executor, clk: clk, idGen: idGen, ownerEpoch: ownerEpoch}
}

// RunOnce finds one admitted episode, fences a worker attempt, executes it,
// and persists the attempt terminal state and proposed Decision.
// It returns true if an episode was processed.
func (r *Runner) RunOnce(ctx context.Context, tenantID string) (bool, error) {
	var req Request
	var episodeID string
	var identity Identity
	var snapshotHash []byte
	var promptHash, objectiveHash []byte
	err := r.withTx(ctx, func(tx *sql.Tx) error {
		if err := tx.QueryRowContext(ctx, `
			SELECT episode_id, scheduler_item_id, tenant_id, situation_id, situation_version,
			       executor_name, executor_version, model_policy, prompt_version,
			       snapshot_sha256, prompt_sha256, objective_sha256, admission_key, request_json
			FROM episodes
			WHERE tenant_id = ? AND lifecycle_status IN ('admitted', 'running')
			ORDER BY accepted_at LIMIT 1`,
			tenantID,
		).Scan(
			&episodeID, &req.SchedulerItemID, &req.TenantID, &req.SituationID, &req.SituationVersion,
			&req.ExecutorName, &req.ExecutorVersion, &req.ModelPolicy, &req.PromptVersion,
			&snapshotHash, &promptHash, &objectiveHash, &req.AdmissionKey, &req.RequestJSON,
		); err != nil {
			if err == sql.ErrNoRows {
				return nil
			}
			return fmt.Errorf("query admitted episode: %w", err)
		}

		req.EpisodeID = episodeID
		req.SnapshotSHA256 = "sha256:" + hex.EncodeToString(snapshotHash)
		req.PromptSHA256 = "sha256:" + hex.EncodeToString(promptHash)
		req.ObjectiveSHA256 = "sha256:" + hex.EncodeToString(objectiveHash)
		var trace struct {
			Traceparent     string `json:"traceparent"`
			Tracestate      string `json:"tracestate"`
			CancellationKey string `json:"cancellation_key"`
			SupersessionKey string `json:"supersession_key"`
		}
		if err := json.Unmarshal(req.RequestJSON, &trace); err != nil {
			return fmt.Errorf("decode persisted request trace context: %w", err)
		}
		if _, err := contractsv1.ParseTraceContext(trace.Traceparent, trace.Tracestate); err != nil {
			return fmt.Errorf("validate persisted request trace context: %w", err)
		}
		req.Traceparent = trace.Traceparent
		req.Tracestate = trace.Tracestate
		req.CancellationKey = trace.CancellationKey
		req.SupersessionKey = trace.SupersessionKey
		entityID, entityErr := requestEntityID(req.RequestJSON)
		if entityErr != nil {
			return fmt.Errorf("load persisted request entity: %w", entityErr)
		}
		req.EntityID = entityID
		req.AttemptID = ""
		attemptID := r.idGen.New(ids.PrefixAttempt)
		var err error
		if r.ownerEpoch != "" {
			identity, err = StartAttemptOwned(ctx, tx, episodeID, attemptID, r.ownerEpoch, r.clk.Now())
		} else {
			identity, err = StartAttempt(ctx, tx, episodeID, attemptID, r.clk.Now())
		}
		if err != nil {
			return fmt.Errorf("start episode attempt: %w", err)
		}
		if err := TransitionAttempt(ctx, tx, identity, AttemptRunning, r.clk.Now(), nil); err != nil {
			return fmt.Errorf("mark episode attempt running: %w", err)
		}
		req.AttemptID = identity.AttemptID
		req.Fence = identity.Fence
		req.RequestJSON, err = bindAttemptIdentity(req.RequestJSON, identity)
		if err != nil {
			return fmt.Errorf("bind worker identity to request: %w", err)
		}
		if _, err := tx.ExecContext(ctx, "UPDATE episodes SET request_json = ? WHERE episode_id = ?", req.RequestJSON, episodeID); err != nil {
			return fmt.Errorf("persist worker request identity: %w", err)
		}
		return nil
	})
	if err != nil {
		return false, err
	}
	if episodeID == "" {
		return false, nil
	}

	executionCtx, stopWatching := context.WithCancel(ctx)
	watchDone := make(chan struct{})
	go r.watchSupersession(executionCtx, episodeID, stopWatching, watchDone)
	outcome, err := r.executor.Execute(executionCtx, &req)
	stopWatching()
	<-watchDone
	if err != nil {
		persistCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		return true, r.failAttemptStatus(persistCtx, identity, executionFailureStatus(err), executionFailureReason(err))
	}
	if outcome == nil {
		persistCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		return true, r.failAttemptStatus(persistCtx, identity, AttemptFailed, "executor_returned_nil_outcome")
	}
	persistCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	if outcome.AttemptID != identity.AttemptID || outcome.Fence != identity.Fence {
		reason := RejectWrongAttempt
		if outcome.Fence < identity.Fence {
			reason = RejectStaleAttempt
		}
		incoming := Identity{EpisodeID: identity.EpisodeID, AttemptID: outcome.AttemptID, Fence: outcome.Fence}
		return true, r.failAttemptWithRejection(ctx, identity, incoming, reason, "worker_identity_mismatch")
	}

	return true, r.withTx(persistCtx, func(tx *sql.Tx) error {
		now := r.clk.Now().UTC().Format(time.RFC3339Nano)
		validationInput, err := decisionInput(&req, identity, r.clk.Now())
		if err != nil {
			return fmt.Errorf("build decision validation input: %w", err)
		}
		validated, validationErr := decisions.Validate(outcome.DecisionJSON, outcome.DecisionSHA256, validationInput)
		decisionID := decisionIDFromJSON(outcome.DecisionJSON)
		if decisionID == "" {
			decisionID = r.idGen.New(ids.PrefixDecision)
		}
		decisionDigest, hasContractDigest := decisionDigestForStorage(outcome.DecisionJSON)
		if outcome.DecisionJSON != nil && !hasContractDigest {
			rawHash := sha256.Sum256(outcome.DecisionJSON)
			decisionDigest = rawHash[:]
		}
		validationStatus := "rejected"
		validationJSON := []byte(`{"reason":"schema_invalid"}`)
		if validationErr == nil {
			validationStatus = "proposed"
			validationJSON = []byte(`{}`)
			if decoded, decodeErr := canonicaljson.DecodeDigest(validated.DecisionDigest); decodeErr == nil {
				decisionDigest = decoded
			}
		} else {
			if typed, ok := validationErr.(*decisions.ValidationError); ok {
				validationJSON, _ = json.Marshal(map[string]any{"reason": typed.Reason, "details": typed.Details})
			} else {
				validationJSON, _ = json.Marshal(map[string]any{"reason": "schema_invalid", "details": validationErr.Error()})
			}
			if !hasContractDigest {
				rawHash := sha256.Sum256(outcome.DecisionJSON)
				var details map[string]any
				_ = json.Unmarshal(validationJSON, &details)
				details["raw_sha256"] = hex.EncodeToString(rawHash[:])
				validationJSON, _ = json.Marshal(details)
			}
		}
		if outcome.DecisionJSON != nil {
			var ordinal int
			if err := tx.QueryRowContext(ctx, "SELECT COALESCE(MAX(ordinal), 0) + 1 FROM decisions WHERE episode_id = ?", episodeID).Scan(&ordinal); err != nil {
				return fmt.Errorf("allocate decision ordinal: %w", err)
			}
			if _, err := tx.ExecContext(ctx, `
				INSERT INTO decisions (
					decision_id, episode_id, attempt_id, fence, ordinal, situation_id,
					situation_version, raw_json, decision_sha256, validation_status,
					validation_json, created_at
				) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
				decisionID, episodeID, identity.AttemptID, identity.Fence, ordinal,
				req.SituationID, req.SituationVersion,
				outcome.DecisionJSON, decisionDigest[:], validationStatus, validationJSON, now,
			); err != nil {
				return fmt.Errorf("insert decision: %w", err)
			}
			if validationErr == nil {
				if err := r.persistValidatedIntents(ctx, tx, validated, &req, now); err != nil {
					return err
				}
				if _, err := tx.ExecContext(ctx, "UPDATE decisions SET validation_status = 'accepted' WHERE decision_id = ?", decisionID); err != nil {
					return fmt.Errorf("accept decision: %w", err)
				}
			} else {
				reason := "schema_invalid"
				if typed, ok := validationErr.(*decisions.ValidationError); ok {
					reason = typed.Reason
				}
				if err := RecordRejection(ctx, tx, identity, RejectionReason(reason), validationJSON, r.clk.Now()); err != nil {
					return fmt.Errorf("record decision rejection: %w", err)
				}
				if _, err := tx.ExecContext(ctx, "UPDATE decisions SET rejection_reason = ? WHERE decision_id = ?", reason, decisionID); err != nil {
					return fmt.Errorf("annotate rejected decision: %w", err)
				}
			}
		}

		attemptStatus := AttemptStatus(outcome.Status)
		if outcome.DecisionJSON != nil {
			if validationErr == nil {
				attemptStatus = AttemptProduced
			} else {
				attemptStatus = AttemptFailed
			}
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
		if r.cost != nil {
			if err := r.cost.Settle(ctx, tx, identity.EpisodeID, outcome.CostMicrounits, now); err != nil {
				return fmt.Errorf("settle episode cost: %w", err)
			}
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

func decisionDigestForStorage(raw []byte) ([]byte, bool) {
	canonical, err := canonicaljson.Marshal(json.RawMessage(raw))
	if err != nil {
		return nil, false
	}
	var document map[string]any
	if err := json.Unmarshal(canonical, &document); err != nil {
		return nil, false
	}
	digest, err := canonicaljson.Digest(canonicaljson.DomainDecision, document)
	if err != nil {
		return nil, false
	}
	decoded, err := canonicaljson.DecodeDigest(digest)
	if err != nil {
		return nil, false
	}
	return decoded, true
}

func bindAttemptIdentity(raw []byte, identity Identity) ([]byte, error) {
	var document map[string]any
	if err := json.Unmarshal(raw, &document); err != nil {
		return nil, fmt.Errorf("decode request json: %w", err)
	}
	document["attempt_id"] = identity.AttemptID
	document["fence"] = identity.Fence
	bound, err := canonicaljson.Marshal(document)
	if err != nil {
		return nil, fmt.Errorf("canonicalize request json: %w", err)
	}
	return bound, nil
}

func decisionInput(req *Request, identity Identity, now time.Time) (decisions.Input, error) {
	var payload struct {
		AllowedIntentTypes []string `json:"allowed_intent_types"`
		RiskCeiling        string   `json:"risk_ceiling"`
	}
	if err := json.Unmarshal(req.RequestJSON, &payload); err != nil {
		return decisions.Input{}, fmt.Errorf("decode request tools: %w", err)
	}
	allowed := make(map[string]struct{}, len(payload.AllowedIntentTypes))
	for _, intentType := range payload.AllowedIntentTypes {
		allowed[intentType] = struct{}{}
	}
	if payload.RiskCeiling == "" {
		return decisions.Input{}, fmt.Errorf("request has no explicit risk ceiling")
	}
	return decisions.Input{
		EpisodeID:          identity.EpisodeID,
		AttemptID:          identity.AttemptID,
		Fence:              identity.Fence,
		TenantID:           req.TenantID,
		SituationID:        req.SituationID,
		SituationVersion:   req.SituationVersion,
		SnapshotDigest:     req.SnapshotSHA256,
		AllowedIntentTypes: allowed,
		RiskCeiling:        payload.RiskCeiling,
		Now:                now,
	}, nil
}

func (r *Runner) persistValidatedIntents(ctx context.Context, tx *sql.Tx, validated *decisions.Result, req *Request, now string) error {
	for _, intent := range validated.Intents {
		digest, err := canonicaljson.DecodeDigest(intent.Digest)
		if err != nil {
			return fmt.Errorf("decode intent digest: %w", err)
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO intents (
				intent_id, decision_id, tenant_id, situation_id, situation_version,
				intent_type, risk_class, intent_json, intent_sha256, expires_at,
				policy_status, created_at, updated_at
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 'pending', ?, ?)`,
			intent.ID, validated.DecisionID, req.TenantID, req.SituationID, req.SituationVersion,
			intent.Type, intent.RiskClass, intent.CanonicalJSON, digest,
			intent.ExpiresAt.UTC().Format(time.RFC3339Nano), now, now,
		); err != nil {
			return fmt.Errorf("insert intent %s: %w", intent.ID, err)
		}
	}
	return nil
}

func decisionIDFromJSON(raw []byte) string {
	var document struct {
		DecisionID string `json:"decision_id"`
	}
	if json.Unmarshal(raw, &document) != nil {
		return ""
	}
	return document.DecisionID
}

func (r *Runner) failAttempt(ctx context.Context, identity Identity, reason string) error {
	return r.failAttemptStatus(ctx, identity, AttemptFailed, reason)
}

func (r *Runner) failAttemptStatus(ctx context.Context, identity Identity, attemptStatus AttemptStatus, reason string) error {
	return r.withTx(ctx, func(tx *sql.Tx) error {
		terminalJSON, err := json.Marshal(map[string]any{"status": attemptStatus, "reason": reason})
		if err != nil {
			return fmt.Errorf("marshal terminal: %w", err)
		}
		if attemptStatus == AttemptCancelled {
			var current AttemptStatus
			if err := tx.QueryRowContext(ctx, "SELECT status FROM episode_attempts WHERE attempt_id = ? AND episode_id = ? AND fence = ?", identity.AttemptID, identity.EpisodeID, identity.Fence).Scan(&current); err != nil {
				return fmt.Errorf("read episode attempt status: %w", err)
			}
			if current != AttemptCancelling {
				if err := TransitionAttempt(ctx, tx, identity, AttemptCancelling, r.clk.Now(), nil); err != nil {
					return fmt.Errorf("mark episode attempt cancelling: %w", err) //nolint:misspell // Durable lifecycle value is frozen as cancelling.
				}
			}
		}
		if err := TransitionAttempt(ctx, tx, identity, attemptStatus, r.clk.Now(), terminalJSON); err != nil {
			return fmt.Errorf("finish failed episode attempt: %w", err)
		}
		var lifecycle string
		if err := tx.QueryRowContext(ctx, "SELECT lifecycle_status FROM episodes WHERE episode_id = ?", identity.EpisodeID).Scan(&lifecycle); err != nil {
			return fmt.Errorf("read episode lifecycle: %w", err)
		}
		var failedAttempts int
		if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM episode_attempts WHERE episode_id = ? AND status IN ('failed', 'timed_out', 'cancelled')`, identity.EpisodeID).Scan(&failedAttempts); err != nil {
			return fmt.Errorf("count failed episode attempts: %w", err)
		}
		if lifecycle != string(LifecycleSuperseded) && failedAttempts < maxEpisodeAttempts {
			if _, err := tx.ExecContext(ctx, `
				UPDATE episodes SET lifecycle_status = 'running', ended_at = NULL, terminal_json = NULL
				WHERE episode_id = ?`,
				identity.EpisodeID,
			); err != nil {
				return fmt.Errorf("update episode failed: %w", err)
			}
		} else {
			if lifecycle != string(LifecycleSuperseded) {
				terminalJSON, err = json.Marshal(map[string]any{"status": AttemptFailed, "reason": "attempt_retry_limit"})
				if err != nil {
					return fmt.Errorf("marshal retry limit terminal: %w", err)
				}
				if _, err := tx.ExecContext(ctx, "UPDATE episodes SET lifecycle_status = 'concluded', ended_at = ?, terminal_json = ? WHERE episode_id = ?", r.clk.Now().UTC().Format(time.RFC3339Nano), terminalJSON, identity.EpisodeID); err != nil {
					return fmt.Errorf("conclude exhausted episode: %w", err)
				}
			}
			if r.cost != nil {
				if err := r.cost.Settle(ctx, tx, identity.EpisodeID, 0, r.clk.Now().UTC().Format(time.RFC3339Nano)); err != nil {
					return fmt.Errorf("settle abandoned episode cost: %w", err)
				}
			}
		}
		return nil
	})
}

func (r *Runner) watchSupersession(ctx context.Context, episodeID string, cancel context.CancelFunc, done chan<- struct{}) {
	defer close(done)
	ticker := time.NewTicker(25 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			var lifecycle string
			if err := r.db.QueryRowContext(ctx, "SELECT lifecycle_status FROM episodes WHERE episode_id = ?", episodeID).Scan(&lifecycle); err == nil && lifecycle == string(LifecycleSuperseded) {
				cancel()
				return
			}
		}
	}
}

func executionFailureStatus(err error) AttemptStatus {
	if errors.Is(err, context.Canceled) || status.Code(err) == codes.Canceled {
		return AttemptCancelled
	}
	if errors.Is(err, context.DeadlineExceeded) || status.Code(err) == codes.DeadlineExceeded {
		return AttemptTimedOut
	}
	return AttemptFailed
}

func executionFailureReason(err error) string {
	var budgetErr *budgetExceededError
	if errors.As(err, &budgetErr) {
		return "budget_exhausted"
	}
	var telemetryErr budgetTelemetryMissingError
	if errors.As(err, &telemetryErr) {
		return "budget_telemetry_missing"
	}
	switch executionFailureStatus(err) {
	case AttemptCancelled:
		return "worker_cancelled" //nolint:misspell // Durable protocol reason is frozen as cancelled.
	case AttemptTimedOut:
		return "worker_deadline_exceeded"
	default:
		return "worker_execution_failed"
	}
}

func (r *Runner) failAttemptWithRejection(ctx context.Context, current, incoming Identity, reason RejectionReason, detail string) error {
	return r.withTx(ctx, func(tx *sql.Tx) error {
		details, err := json.Marshal(map[string]any{"message": detail, "incoming_attempt_id": incoming.AttemptID, "incoming_fence": incoming.Fence})
		if err != nil {
			return fmt.Errorf("marshal identity rejection: %w", err)
		}
		if err := RecordRejection(ctx, tx, incoming, reason, details, r.clk.Now()); err != nil {
			return fmt.Errorf("record worker identity rejection: %w", err)
		}
		terminalJSON, err := json.Marshal(map[string]any{"status": AttemptFailed, "reason": detail})
		if err != nil {
			return fmt.Errorf("marshal terminal: %w", err)
		}
		if err := TransitionAttempt(ctx, tx, current, AttemptFailed, r.clk.Now(), terminalJSON); err != nil {
			return fmt.Errorf("finish identity-failed attempt: %w", err)
		}
		var failedAttempts int
		if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM episode_attempts WHERE episode_id = ? AND status IN ('failed', 'timed_out', 'cancelled')`, current.EpisodeID).Scan(&failedAttempts); err != nil {
			return fmt.Errorf("count identity-failed attempts: %w", err)
		}
		if failedAttempts >= maxEpisodeAttempts {
			if r.cost != nil {
				if err := r.cost.Settle(ctx, tx, current.EpisodeID, 0, r.clk.Now().UTC().Format(time.RFC3339Nano)); err != nil {
					return fmt.Errorf("settle exhausted episode cost: %w", err)
				}
			}
			if _, err := tx.ExecContext(ctx, "UPDATE episodes SET lifecycle_status = 'concluded', ended_at = ?, terminal_json = ? WHERE episode_id = ?", r.clk.Now().UTC().Format(time.RFC3339Nano), terminalJSON, current.EpisodeID); err != nil {
				return fmt.Errorf("conclude identity-failed episode: %w", err)
			}
		} else if _, err := tx.ExecContext(ctx, `
			UPDATE episodes SET lifecycle_status = 'running', ended_at = NULL, terminal_json = NULL
			WHERE episode_id = ?`, current.EpisodeID); err != nil {
			return fmt.Errorf("retain episode for retry: %w", err)
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
