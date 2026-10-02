package episodes

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/canonicaljson"
	"github.com/ghassan-ai-projects/agentic-stream/internal/clock"
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
	claim, err := r.claimEpisode(ctx, tenantID)
	if err != nil {
		return false, err
	}
	if claim == nil {
		return false, nil
	}
	// A quarantined episode was committed inside the claim transaction:
	// report it processed so the batch continues past it.
	if claim.quarantined {
		return true, nil
	}
	// A re-bind is counted only after its transaction committed. A later
	// in-transaction failure rolls it back and must not bump the counter.
	if claim.rebound && r.telemetry != nil {
		r.telemetry.ObserveStaleRebind()
	}

	startedAt := r.clk.Now()
	outcome, executionErr := r.executeClaim(ctx, claim)
	if r.telemetry != nil {
		r.telemetry.ObserveDuration(r.clk.Now().Sub(startedAt))
	}
	persistCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	return true, r.recordExecution(persistCtx, claim, outcome, executionErr, r.deadlineExceeded(&claim.req, startedAt))
}

// executeClaim runs the fenced attempt under a supersession watch, so a
// superseded episode cancels its provider call.
func (r *Runner) executeClaim(ctx context.Context, claim *episodeClaim) (outcome *Outcome, executionErr error) {
	executionCtx, stopWatching := context.WithCancel(ctx)
	watchDone := make(chan struct{})
	go r.watchSupersession(executionCtx, claim.episodeID, stopWatching, watchDone)
	defer func() {
		stopWatching()
		<-watchDone
	}()

	_, span := telemetry.StartSpan(executionCtx, "agentic_stream.episode.execute")
	telemetry.AddLinkFromW3C(span, claim.req.Traceparent, claim.req.Tracestate)
	defer func() {
		if executionErr != nil {
			telemetry.RecordError(span, executionErr)
		}
		span.End()
	}()
	outcome, executionErr = r.executor.Execute(executionCtx, &claim.req)
	if executionErr != nil {
		// Wrapped errors keep their gRPC status and budget types; the
		// failure classifiers unwrap with errors.Is/As and status.Code.
		return nil, fmt.Errorf("execute episode attempt: %w", executionErr)
	}
	return outcome, nil
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

func (r *Runner) withTx(ctx context.Context, fn func(*sql.Tx) error) error {
	if err := r.db.WithTx(ctx, fn); err != nil {
		return fmt.Errorf("tx: %w", err)
	}
	return nil
}
