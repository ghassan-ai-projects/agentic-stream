package episodes

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/canonicaljson"
	"github.com/ghassan-ai-projects/agentic-stream/internal/clock"
	"github.com/ghassan-ai-projects/agentic-stream/internal/contractsv1"
	"github.com/ghassan-ai-projects/agentic-stream/internal/costcontrol"
	"github.com/ghassan-ai-projects/agentic-stream/internal/decisions"
	"github.com/ghassan-ai-projects/agentic-stream/internal/ids"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
	"github.com/ghassan-ai-projects/agentic-stream/internal/telemetry"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// Executor runs a bounded episode against an Episode Request and returns a
// terminal outcome. Implementations must not mutate stream or action state
// directly; all effects are returned as typed Decisions and Intents.
type Executor interface {
	// Execute runs the episode to completion or budget exhaustion.
	Execute(ctx context.Context, req *Request) (*Outcome, error)
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
	db           *storage.DB
	executor     Executor
	clk          clock.Clock
	idGen        ids.Generator
	ownerEpoch   string
	cost         *costcontrol.Controller
	epochControl *storage.EpochControl
	shadowStore  *storage.ShadowStore
	telemetry    *telemetry.Runtime
	assembler    *Assembler
}

const maxEpisodeAttempts = 3

// maxStaleRebinds bounds how many times an admitted episode may be re-bound to
// a newer live situation version before it is abandoned as stale. Dense
// entities churn versions faster than the dispatch poll; the durable
// stale_rebind_count column tracks the budget across batches and restarts.
const maxStaleRebinds = 3

// WithCostControl enables settlement of durable episode cost reservations.
func (r *Runner) WithCostControl(controller *costcontrol.Controller) *Runner {
	r.cost = controller
	return r
}

// WithEpochControl enables the P8 kill gate: every dispatch validates the
// episode's RECORDED policy epoch against the control table, so a killed
// epoch refuses in-flight decisions independently of the worker.
func (r *Runner) WithEpochControl(control *storage.EpochControl) *Runner {
	r.epochControl = control
	return r
}

// WithShadowStore enables P8 shadow scoring: shadow decisions are scored
// and persisted to shadow_decisions (never to intents/commands).
func (r *Runner) WithShadowStore(store *storage.ShadowStore) *Runner {
	r.shadowStore = store
	return r
}

// WithTelemetry enables the P8 freshness/latency surface: stale-decision
// rejections and dispatch→decision durations feed the /metrics percentiles.
func (r *Runner) WithTelemetry(telemetry *telemetry.Runtime) *Runner {
	r.telemetry = telemetry
	return r
}

// WithAssembler enables the ISSUE-061 re-bind path: an admitted episode whose
// situation advanced past its bound version is re-bound to the live version
// and dispatched instead of abandoned. Without an assembler the runner keeps
// the pre-fix abandon behavior (tests and minimal wiring).
func (r *Runner) WithAssembler(assembler *Assembler) *Runner {
	r.assembler = assembler
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
	var staleQuarantined bool
	var rebindCount int
	rebound := false
	quarantined := false
	lifecyclePredicate := "lifecycle_status IN ('admitted', 'running')"
	if r.epochControl != nil {
		// Kill supersedes admitted episodes that have not started an attempt.
		// Include only those rows so the runner can release their reservation
		// and durably quarantine them; other superseded episodes are terminal
		// for a different reason and must not be dispatched again.
		lifecyclePredicate += ` OR (lifecycle_status = 'superseded' AND current_attempt_id IS NULL
			AND EXISTS (SELECT 1 FROM epoch_control WHERE epoch = episodes.policy_epoch AND state = 'killed'))`
	}
	err := r.withTx(ctx, func(tx *sql.Tx) error {
		query := fmt.Sprintf(`
			SELECT episode_id, scheduler_item_id, tenant_id, situation_id, situation_version,
			       executor_name, executor_version, model_policy, prompt_version,
			       snapshot_sha256, prompt_sha256, objective_sha256, admission_key, request_json,
			       dispatch_policy, policy_epoch, stale_rebind_count
			FROM episodes
			WHERE tenant_id = ? AND (%s)
			ORDER BY accepted_at, episode_id LIMIT 1`, lifecyclePredicate)
		if err := tx.QueryRowContext(ctx, query,
			tenantID,
		).Scan(
			&episodeID, &req.SchedulerItemID, &req.TenantID, &req.SituationID, &req.SituationVersion,
			&req.ExecutorName, &req.ExecutorVersion, &req.ModelPolicy, &req.PromptVersion,
			&snapshotHash, &promptHash, &objectiveHash, &req.AdmissionKey, &req.RequestJSON,
			&req.DispatchPolicy, &req.PolicyEpoch, &rebindCount,
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
		if _, err := req.WallTimeBudget(); err != nil {
			return fmt.Errorf("validate persisted episode budget: %w", err)
		}
		entityID, entityErr := requestEntityID(req.RequestJSON)
		if entityErr != nil {
			return fmt.Errorf("load persisted request entity: %w", entityErr)
		}
		req.EntityID = entityID
		// P8 (freshness): the situation version is rechecked immediately
		// before dispatch. If a newer version is live, this episode's snapshot
		// is stale — refuse dispatch rather than reason over old facts.
		var liveVersion int64
		if err := tx.QueryRowContext(ctx, `
			SELECT current_version FROM situations
			WHERE tenant_id = ? AND situation_id = ?`,
			req.TenantID, req.SituationID,
		).Scan(&liveVersion); err != nil {
			return fmt.Errorf("recheck live situation version: %w", err)
		}
		if liveVersion != int64(req.SituationVersion) {
			// ISSUE-061: re-bind the stale episode to the live version instead
			// of abandoning it — the decision then reflects the freshest state.
			// Bounded by maxStaleRebinds via the durable counter; when the
			// budget is exhausted or the live snapshot cannot be validated, the
			// episode is quarantined durably (committed in THIS tx — an error
			// return would roll it back) so it cannot block the admitted queue
			// forever (it is always the oldest admitted row). A quarantine
			// returns nil here; only a successful re-bind falls through to
			// dispatch below.
			if r.assembler != nil && rebindCount < maxStaleRebinds {
				fresh, rebindErr := r.assembler.Rebind(ctx, tx, &req, int(liveVersion))
				if rebindErr != nil {
					// The reachable causes are DB corruption or a validation
					// bug — the engine validates at publish. Log loudly and
					// count under rebind_failures, distinct from the benign
					// stale_rejections counter. The failed attempt still
					// consumes re-bind budget so the bounded path is reachable.
					slog.ErrorContext(ctx, "episode re-bind failed: live snapshot invalid",
						"episode_id", episodeID,
						"bound", req.SituationVersion, "live", liveVersion,
						"error", rebindErr.Error())
					terminal, marshalErr := json.Marshal(map[string]any{"reason": "rebind_failed",
						"bound": req.SituationVersion, "live": liveVersion,
						"rebind_attempts": rebindCount + 1, "error": rebindErr.Error()})
					if marshalErr != nil {
						return fmt.Errorf("marshal rebind terminal: %w", marshalErr)
					}
					if _, execErr := tx.ExecContext(ctx, `
							UPDATE episodes SET lifecycle_status = 'abandoned', ended_at = ?, terminal_json = ?,
							    stale_rebind_count = stale_rebind_count + 1
							WHERE episode_id = ?`,
						r.clk.Now().UTC().Format(time.RFC3339Nano), terminal, episodeID); execErr != nil {
						return fmt.Errorf("quarantine rebind-failed episode: %w", execErr)
					}
					if r.telemetry != nil {
						r.telemetry.ObserveRebindFailure()
					}
					staleQuarantined = true
					return nil
				}
				req = *fresh
				liveHash, err := canonicaljson.DecodeDigest(req.SnapshotSHA256)
				if err != nil {
					return fmt.Errorf("decode re-bound snapshot digest: %w", err)
				}
				if _, err := tx.ExecContext(ctx, `
					UPDATE episodes SET situation_version = ?, snapshot_sha256 = ?, request_json = ?,
					    stale_rebind_count = stale_rebind_count + 1
					WHERE episode_id = ?`,
					req.SituationVersion, liveHash, req.RequestJSON, episodeID); err != nil {
					return fmt.Errorf("persist episode re-bind: %w", err)
				}
				rebound = true
				// Bound == live inside this tx (writes are serialized), so the
				// dispatch below reasons over the freshest committed state.
			} else {
				terminal, marshalErr := json.Marshal(map[string]any{"reason": "stale_situation",
					"bound": req.SituationVersion, "live": liveVersion, "rebind_attempts": rebindCount})
				if marshalErr != nil {
					return fmt.Errorf("marshal stale terminal: %w", marshalErr)
				}
				if _, execErr := tx.ExecContext(ctx, `
					UPDATE episodes SET lifecycle_status = 'abandoned', ended_at = ?, terminal_json = ?
					WHERE episode_id = ?`,
					r.clk.Now().UTC().Format(time.RFC3339Nano), terminal, episodeID); execErr != nil {
					return fmt.Errorf("quarantine stale episode: %w", execErr)
				}
				if r.telemetry != nil {
					r.telemetry.ObserveStaleRejection()
				}
				staleQuarantined = true
				return nil
			}
		}
		// P8 (kill): a decision under a killed policy epoch is refused
		// INDEPENDENTLY of the worker — the episode's recorded epoch is
		// checked at the dispatch boundary, so a hostile worker cannot slip a
		// decision through after the kill.
		if r.epochControl != nil {
			epochErr := storage.ErrEpochUnbound
			if req.PolicyEpoch != "" {
				epochErr = r.epochControl.AssertDecisionTx(ctx, tx, req.PolicyEpoch)
			}
			if epochErr != nil {
				if !errors.Is(epochErr, storage.ErrEpochKilled) && !errors.Is(epochErr, storage.ErrEpochUnbound) {
					return fmt.Errorf("check episode policy epoch: %w", epochErr)
				}
				reason := "epoch_killed"
				if errors.Is(epochErr, storage.ErrEpochUnbound) {
					reason = "epoch_unbound"
				}
				terminal, marshalErr := json.Marshal(map[string]any{"reason": reason})
				if marshalErr != nil {
					return fmt.Errorf("marshal epoch terminal: %w", marshalErr)
				}
				if _, execErr := tx.ExecContext(ctx, `
					UPDATE episodes SET lifecycle_status = 'abandoned', ended_at = ?, terminal_json = ?
					WHERE episode_id = ?`,
					r.clk.Now().UTC().Format(time.RFC3339Nano), terminal, episodeID); execErr != nil {
					return fmt.Errorf("quarantine killed-epoch episode: %w", execErr)
				}
				if r.cost != nil {
					if settleErr := r.cost.Settle(ctx, tx, episodeID, 0, r.clk.Now().UTC().Format(time.RFC3339Nano)); settleErr != nil {
						return fmt.Errorf("settle quarantined episode cost: %w", settleErr)
					}
				}
				quarantined = true
				return nil
			}
		}
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
	// A stale episode was quarantined (committed) inside the tx — report it
	// processed and let the batch continue; the queue drains past it.
	if staleQuarantined {
		return true, nil
	}
	if quarantined {
		return true, nil
	}
	// A successful re-bind is counted only after its tx committed — a later
	// in-tx failure (attempt start, identity binding) rolls the re-bind back
	// and must not bump the recovery counter.
	if rebound && r.telemetry != nil {
		r.telemetry.ObserveStaleRebind()
	}

	executionCtx, stopWatching := context.WithCancel(ctx)
	watchDone := make(chan struct{})
	go r.watchSupersession(executionCtx, episodeID, stopWatching, watchDone)
	var outcome *Outcome
	var executionErr error
	startedAt := r.clk.Now()
	func() {
		_, span := telemetry.StartSpan(executionCtx, "agentic_stream.episode.execute")
		telemetry.AddLinkFromW3C(span, req.Traceparent, req.Tracestate)
		defer func() {
			if executionErr != nil {
				telemetry.RecordError(span, executionErr)
			}
			span.End()
		}()
		outcome, executionErr = r.executor.Execute(executionCtx, &req)
	}()
	if r.telemetry != nil {
		r.telemetry.ObserveDuration(r.clk.Now().Sub(startedAt))
	}
	err = executionErr
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
		return true, r.failAttemptWithRejection(persistCtx, identity, incoming, reason, "worker_identity_mismatch")
	}
	// P8 (freshness): a decision that arrives after the episode's wall_time
	// deadline is refused (terminal timed_out) — the deadline is never extended
	// to let a slow model pass, and a hostile in-process executor cannot
	// bypass the gate either.
	if r.deadlineExceeded(&req, startedAt) {
		return true, r.failAttemptStatus(persistCtx, identity, AttemptTimedOut, "decision_after_deadline")
	}

	return true, r.withTx(persistCtx, func(tx *sql.Tx) error {
		now := r.clk.Now().UTC().Format(time.RFC3339Nano)
		// P8 (kill, post-execute): the recorded epoch is re-asserted INSIDE
		// the persistence transaction — an attempt dispatched before the kill
		// that completes after it is refused here, so a hostile worker cannot
		// slip a decision into governance. The episode is quarantined.
		if r.epochControl != nil {
			epochErr := storage.ErrEpochUnbound
			if req.PolicyEpoch != "" {
				epochErr = r.epochControl.AssertDecisionTx(persistCtx, tx, req.PolicyEpoch)
			}
			if epochErr != nil {
				if !errors.Is(epochErr, storage.ErrEpochKilled) && !errors.Is(epochErr, storage.ErrEpochUnbound) {
					return fmt.Errorf("check post-execute policy epoch: %w", epochErr)
				}
				reason := "epoch_killed_post_execute"
				if errors.Is(epochErr, storage.ErrEpochUnbound) {
					reason = "epoch_unbound_post_execute"
				}
				terminal, marshalErr := json.Marshal(map[string]any{"reason": reason})
				if marshalErr != nil {
					return fmt.Errorf("marshal post-execute epoch terminal: %w", marshalErr)
				}
				attemptTerminal, marshalErr := json.Marshal(map[string]any{
					"status": string(AttemptAbandoned),
					"reason": reason,
				})
				if marshalErr != nil {
					return fmt.Errorf("marshal post-execute attempt terminal: %w", marshalErr)
				}
				if err := TransitionAttempt(persistCtx, tx, identity, AttemptAbandoned, r.clk.Now(), attemptTerminal); err != nil {
					return fmt.Errorf("abandon killed epoch attempt: %w", err)
				}
				if _, execErr := tx.ExecContext(persistCtx, `
					UPDATE episodes SET lifecycle_status = 'abandoned', ended_at = ?, terminal_json = ?
					WHERE episode_id = ?`,
					now, terminal, episodeID); execErr != nil {
					return fmt.Errorf("quarantine post-execute killed-epoch episode: %w", execErr)
				}
				if r.cost != nil {
					if settleErr := r.cost.Settle(persistCtx, tx, episodeID, outcome.CostMicrounits, now); settleErr != nil {
						return fmt.Errorf("settle post-execute quarantined episode cost: %w", settleErr)
					}
				}
				quarantined = true
				return nil
			}
		}
		var validated *decisions.Result
		var validationErr error
		if outcome.DecisionJSON != nil {
			validationInput, err := decisionInput(&req, identity, r.clk.Now())
			if err != nil {
				return fmt.Errorf("build decision validation input: %w", err)
			}
			validated, validationErr = decisions.Validate(outcome.DecisionJSON, outcome.DecisionSHA256, validationInput)
		}
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
		var validationJSON []byte
		if outcome.DecisionJSON != nil {
			if validationErr == nil {
				validationStatus = "proposed"
				validationJSON = []byte(`{}`)
				if decoded, decodeErr := canonicaljson.DecodeDigest(validated.DecisionDigest); decodeErr == nil {
					decisionDigest = decoded
				}
			} else {
				var typed *decisions.ValidationError
				if errors.As(validationErr, &typed) {
					validationJSON, err = json.Marshal(map[string]any{"reason": typed.Reason, "details": typed.Details})
				} else {
					validationJSON, err = json.Marshal(map[string]any{"reason": "schema_invalid", "details": validationErr.Error()})
				}
				if err != nil {
					return fmt.Errorf("marshal decision validation: %w", err)
				}
				if !hasContractDigest {
					rawHash := sha256.Sum256(outcome.DecisionJSON)
					var details map[string]any
					if err := json.Unmarshal(validationJSON, &details); err != nil {
						return fmt.Errorf("decode decision validation: %w", err)
					}
					details["raw_sha256"] = hex.EncodeToString(rawHash[:])
					validationJSON, err = json.Marshal(details)
					if err != nil {
						return fmt.Errorf("marshal raw decision validation: %w", err)
					}
				}
			}
		}
		if outcome.DecisionJSON != nil {
			var ordinal int
			if err := tx.QueryRowContext(persistCtx, "SELECT COALESCE(MAX(ordinal), 0) + 1 FROM decisions WHERE episode_id = ?", episodeID).Scan(&ordinal); err != nil {
				return fmt.Errorf("allocate decision ordinal: %w", err)
			}
			if _, err := tx.ExecContext(persistCtx, `
				INSERT INTO decisions (
					decision_id, episode_id, attempt_id, fence, ordinal, situation_id,
					situation_version, raw_json, decision_sha256, validation_status,
					validation_json, traceparent, tracestate, created_at
				) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
				decisionID, episodeID, identity.AttemptID, identity.Fence, ordinal,
				req.SituationID, req.SituationVersion,
				outcome.DecisionJSON, decisionDigest[:], validationStatus, validationJSON,
				nullableString(req.Traceparent), nullableString(req.Tracestate), now,
			); err != nil {
				return fmt.Errorf("insert decision: %w", err)
			}
			if validationErr == nil {
				if req.DispatchPolicy == "shadow" {
					// P8 (shadow-first): a shadow decision is scored — the
					// would-be policy outcome is computed from the intents —
					// but NOTHING is written to intents or commands. Shadow
					// never enters action governance.
					if err := r.recordShadow(persistCtx, tx, decisionID, decisionDigest, &req, outcome, validated, now); err != nil {
						return err
					}
				} else {
					if err := r.persistValidatedIntents(persistCtx, tx, validated, &req, now); err != nil {
						return err
					}
				}
				if _, err := tx.ExecContext(persistCtx, "UPDATE decisions SET validation_status = 'accepted' WHERE decision_id = ?", decisionID); err != nil {
					return fmt.Errorf("accept decision: %w", err)
				}
			} else {
				reason := "schema_invalid"
				var typed *decisions.ValidationError
				if errors.As(validationErr, &typed) {
					reason = typed.Reason
				}
				if err := RecordRejection(persistCtx, tx, identity, RejectionReason(reason), validationJSON, r.clk.Now()); err != nil {
					return fmt.Errorf("record decision rejection: %w", err)
				}
				if _, err := tx.ExecContext(persistCtx, "UPDATE decisions SET rejection_reason = ? WHERE decision_id = ?", reason, decisionID); err != nil {
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
		if err := TransitionAttempt(persistCtx, tx, identity, attemptStatus, r.clk.Now(), terminalJSON); err != nil {
			return fmt.Errorf("finish episode attempt: %w", err)
		}
		if r.cost != nil {
			if err := r.cost.Settle(persistCtx, tx, identity.EpisodeID, outcome.CostMicrounits, now); err != nil {
				return fmt.Errorf("settle episode cost: %w", err)
			}
		}
		if _, err := tx.ExecContext(persistCtx, `
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
		Kind               string   `json:"kind"`
		Executor           struct {
			IntentCatalog       []map[string]any `json:"intent_catalog"`
			IntentCatalogSHA256 string           `json:"intent_catalog_sha256"`
		} `json:"executor"`
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
	// P4/B10: the catalog is verified INDEPENDENTLY at the validation
	// boundary — the digest must bind the parsed bytes under the shared
	// domain; a forged/missing/empty catalog fails closed before the decision
	// is trusted (the worker's own verify_wire is not evidence here).
	if !canonicaljson.Verify(canonicaljson.DomainIntentCatalog, payload.Executor.IntentCatalog, payload.Executor.IntentCatalogSHA256) {
		return decisions.Input{}, fmt.Errorf("intent catalog is missing, forged, or malformed")
	}
	compiled, err := decisions.CompileIntentCatalog(payload.Executor.IntentCatalog)
	if err != nil {
		return decisions.Input{}, fmt.Errorf("compile intent catalog: %w", err)
	}
	return decisions.Input{
		EpisodeID:          identity.EpisodeID,
		AttemptID:          identity.AttemptID,
		Fence:              identity.Fence,
		TenantID:           req.TenantID,
		SituationID:        req.SituationID,
		SituationVersion:   req.SituationVersion,
		EntityID:           req.EntityID,
		SnapshotDigest:     req.SnapshotSHA256,
		AllowedIntentTypes: allowed,
		RiskCeiling:        payload.RiskCeiling,
		IntentCatalog:      compiled,
		Kind:               payload.Kind,
		Now:                now,
	}, nil
}

func nullableString(value string) sql.NullString {
	return sql.NullString{String: value, Valid: value != ""}
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
				rate_limit_per_hour, requires_approval, policy_status, created_at, updated_at
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 'pending', ?, ?)`,
			intent.ID, validated.DecisionID, req.TenantID, req.SituationID, req.SituationVersion,
			intent.Type, intent.RiskClass, intent.CanonicalJSON, digest,
			intent.ExpiresAt.UTC().Format(time.RFC3339Nano), intent.RateLimitPerHour,
			boolToInt(intent.RequiresApproval), now, now,
		); err != nil {
			return fmt.Errorf("insert intent %s: %w", intent.ID, err)
		}
	}
	return nil
}

func boolToInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

// recordShadow scores a shadow decision: the would-be policy outcome computed
// from the validated intents, persisted ONLY to the shadow_decisions table —
// never to intents or commands. The score is the highest-risk intent's would-be
// result under the live policy (R0/R1 automatic, R2 requires approval, R3/R4
// denied). The decision is correlated by decision_id, so a shadow row is
// traceable to the decision it scored.
func (r *Runner) recordShadow(ctx context.Context, tx *sql.Tx, decisionID string, decisionDigest []byte, req *Request, outcome *Outcome, validated *decisions.Result, now string) error {
	// decisions.Validate rejects a decision with zero intents, so the first
	// intent is always present here.
	highest := validated.Intents[0]
	for _, intent := range validated.Intents[1:] {
		if intentRiskRanks[intent.RiskClass] > intentRiskRanks[highest.RiskClass] {
			highest = intent
		}
	}
	var score storage.ShadowScore
	var reason string
	switch highest.RiskClass {
	case "R0", "R1":
		score = storage.ShadowWouldApprove
		reason = "would_approve_" + highest.RiskClass
	case "R2":
		score = storage.ShadowWouldRequireApproval
		reason = "would_require_approval_r2"
	default:
		score = storage.ShadowWouldDeny
		reason = "would_deny_" + highest.RiskClass
	}
	decisionSHA := decisionDigest
	if r.shadowStore == nil {
		return fmt.Errorf("shadow dispatch but no shadow store configured — scores would be silently dropped")
	}
	shadow := storage.ShadowDecision{
		ShadowDecisionID: r.idGen.New(ids.PrefixShadow),
		EpisodeID:        req.EpisodeID,
		DecisionID:       decisionID,
		AttemptID:        req.AttemptID,
		Fence:            req.Fence,
		DecisionJSON:     outcome.DecisionJSON,
		DecisionSHA256:   decisionSHA,
		ShadowScore:      score,
		ScoreReason:      reason,
		TenantID:         req.TenantID,
		SituationID:      req.SituationID,
		SituationVersion: req.SituationVersion,
		PolicyEpoch:      req.PolicyEpoch,
	}
	if err := r.shadowStore.Record(ctx, tx, shadow, now); err != nil {
		return fmt.Errorf("record shadow decision: %w", err)
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

// deadlineExceeded reports whether the attempt ran past its wall_time budget.
// The budget is the executor's canonical input (never extended after
// admission); a decision produced after it is refused.
func (r *Runner) deadlineExceeded(req *Request, startedAt time.Time) bool {
	wallTime, err := req.WallTimeBudget()
	if err != nil || wallTime <= 0 {
		return false
	}
	return r.clk.Now().After(startedAt.Add(wallTime))
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
