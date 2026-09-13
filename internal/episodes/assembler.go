// Package episodes assembles deterministic episode requests from scheduler
// items and persists episode records for execution.
package episodes

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/canonicaljson"
	"github.com/ghassan-ai-projects/agentic-stream/internal/contractsv1"
	"github.com/ghassan-ai-projects/agentic-stream/internal/costcontrol"
	"github.com/ghassan-ai-projects/agentic-stream/internal/ids"
	"github.com/ghassan-ai-projects/agentic-stream/internal/spec"
	"modernc.org/sqlite"
	sqlite3 "modernc.org/sqlite/lib"
)

// ErrLiveEpisodeConflict means a reconsideration could not be admitted
// because another episode for the same Situation is already live.
var ErrLiveEpisodeConflict = errors.New("one live episode per situation constraint")

// Request is the durable input to an episode executor. Its persistence fields
// map to the episodes table; RequestJSON is the canonical executor input.
type Request struct {
	EpisodeID        string // unique episode identity.
	SchedulerItemID  string // scheduler item that admitted this episode.
	Kind             string // standard or reconsider.
	TenantID         string // tenant owning the situation.
	SituationID      string // situation being reasoned about.
	SituationVersion int    // immutable situation version bound to this episode.
	EntityID         string // entity bound to the immutable situation snapshot.
	ExecutorName     string // executor configured in the active spec.
	ExecutorVersion  string // digest of the active spec.
	ModelPolicy      string // model policy from the spec executor.
	PromptVersion    string // prompt version from the spec executor.
	PromptSHA256     string // content-addressed prompt reference.
	ObjectiveSHA256  string // content-addressed objective.
	SnapshotSHA256   string // deterministic hash of the snapshot subset.
	AttemptID        string // worker attempt identity, set at dispatch.
	Fence            int64  // worker fence, set at dispatch.
	AdmissionKey     []byte // unique 32-byte admission key.
	RequestJSON      []byte // canonical JSON sent to the executor.
	// wallTime is populated once from RequestJSON by WallTimeBudget. Keeping the
	// parsed value on the request lets every execution path share one boundary
	// validation without reparsing durable JSON.
	wallTime          time.Duration
	wallTimeValidated bool
	Traceparent       string
	Tracestate        string
	CancellationKey   string
	SupersessionKey   string
	// P8: the mode matrix. DispatchPolicy is active|shadow (from the spec);
	// PolicyEpoch is the runtime owner epoch the episode was admitted under —
	// set ONCE, never rewritten, so a drained epoch refuses only new admission
	// and only a killed epoch refuses in-flight.
	DispatchPolicy string
	PolicyEpoch    string
}

// Assembler builds deterministic episode requests.
type Assembler struct {
	spec  *spec.CompiledSpec
	idGen ids.Generator
	cost  *costcontrol.Controller
}

// WithCostControl enables durable aggregate cost reservation at admission.
func (a *Assembler) WithCostControl(controller *costcontrol.Controller) *Assembler {
	a.cost = controller
	return a
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

	evidence, err := a.loadValidatedSnapshot(ctx, tx, item.SituationID, item.SituationVersion, tenantID)
	if err != nil {
		return nil, err
	}
	snapshot := evidence.document
	entityID := evidence.entityID
	traceparent := evidence.traceparent
	tracestate := evidence.tracestate

	var delta map[string]any
	if len(ev.DeltaJSON) > 0 {
		if err := json.Unmarshal(ev.DeltaJSON, &delta); err != nil {
			return nil, fmt.Errorf("unmarshal delta: %w", err)
		}
	}

	var reconsideration map[string]any
	if item.Kind == "reconsider" {
		reconsideration, err = loadReconsideration(ctx, tx, item, ev, delta, snapshot)
		if err != nil {
			return nil, fmt.Errorf("load reconsideration: %w", err)
		}
	}

	tools := a.buildTools()
	episodeID := a.idGen.New(ids.PrefixEpisode)
	request := map[string]any{
		"episode_id":        episodeID,
		"kind":              item.Kind,
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
		"snapshot":               snapshot,
		"delta":                  delta,
		"tools":                  tools,
		"allowed_intent_types":   a.allowedIntentTypeList(),
		"watch_confidence_floor": a.spec.Actions.EffectiveWatchConfidenceFloor(),
		"risk_ceiling":           a.effectiveRiskCeiling(),
		"executor": map[string]any{
			"name":              a.spec.Cognition.Executor.Name,
			"model_policy":      a.spec.Cognition.Executor.ModelPolicy,
			"prompt_version":    a.spec.Cognition.Executor.PromptVersion,
			"prompt":            a.spec.Cognition.Executor.Prompt,
			"objective":         a.spec.Cognition.Executor.Objective,
			"decision_schema":   a.spec.Cognition.Executor.DecisionSchema,
			"diagnosis_catalog": a.spec.Cognition.Executor.DiagnosisCatalog,
		},
		"budget":           a.budgetMap(),
		"cancellation_key": "episode:" + episodeID,
		"supersession_key": "situation:" + item.SituationID,
		"traceparent":      traceparent,
		"tracestate":       tracestate,
	}
	if reconsideration != nil {
		request["reconsideration"] = reconsideration
	}
	promptDigest, err := canonicaljson.Digest(canonicaljson.DomainPrompt, map[string]any{
		"version": a.spec.Cognition.Executor.PromptVersion,
		"text":    a.spec.Cognition.Executor.Prompt,
	})
	if err != nil {
		return nil, fmt.Errorf("digest prompt provenance: %w", err)
	}
	objectiveDigest, err := canonicaljson.Digest(canonicaljson.DomainObjective, map[string]any{"text": a.spec.Cognition.Executor.Objective})
	if err != nil {
		return nil, fmt.Errorf("digest objective provenance: %w", err)
	}
	// P1: the diagnosis catalog digest binds the catalog document the Ruby
	// worker verifies (shared situation-runtime/diagnosis-catalog domain).
	// The digest is over the PARSED catalog (the array shape), matching
	// DiagnosisCatalog.verify_wire — a wrapped-string shape would digest
	// differently and every Go-driven episode would fail closed in the Ruby
	// worker. An invalid catalog document fails compilation.
	var catalogValue any = []any{}
	if strings.TrimSpace(a.spec.Cognition.Executor.DiagnosisCatalog) != "" {
		if err := json.Unmarshal([]byte(a.spec.Cognition.Executor.DiagnosisCatalog), &catalogValue); err != nil {
			return nil, fmt.Errorf("diagnosis catalog is not valid JSON: %w", err)
		}
	}
	catalogDigest, err := canonicaljson.Digest(canonicaljson.DomainDiagnosisCatalog, catalogValue)
	if err != nil {
		return nil, fmt.Errorf("digest diagnosis catalog provenance: %w", err)
	}
	executorDocument := request["executor"].(map[string]any)
	executorDocument["prompt_sha256"] = promptDigest
	executorDocument["objective_sha256"] = objectiveDigest
	executorDocument["diagnosis_catalog_sha256"] = catalogDigest

	// P4: the intent catalog is compiled from the spec and embedded with its
	// shared-domain digest — the Ruby worker verifies it via
	// IntentCatalog.verify_wire before any model call, and the Go validator
	// verifies it again independently (B10). A missing, empty, duplicate, or
	// structurally invalid catalog fails compilation.
	intentCatalog, intentCatalogDigest, err := CompileIntentCatalog(a.spec.Actions.Intents)
	if err != nil {
		return nil, fmt.Errorf("compile intent catalog: %w", err)
	}
	executorDocument["intent_catalog"] = intentCatalog
	executorDocument["intent_catalog_sha256"] = intentCatalogDigest

	// P5: the digest-pinned skill refs flow to the worker, which resolves the
	// text only from the operator-approved directory and requires the tree
	// digest to match (unknown name or mismatch fails before a model call).
	executorDocument["skill_refs"] = a.spec.Cognition.Executor.Skills

	admissionKey := sha256.Sum256([]byte(episodeID + "|" + schedulerItemID))

	// The snapshot digest covers exactly the immutable Situation snapshot, not
	// trigger routing or executor capabilities.
	request["snapshot_digest"] = evidence.digest
	requestJSON, err := canonicaljson.Marshal(request)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	req := &Request{
		EpisodeID:        episodeID,
		SchedulerItemID:  schedulerItemID,
		Kind:             item.Kind,
		TenantID:         tenantID,
		SituationID:      item.SituationID,
		SituationVersion: item.SituationVersion,
		EntityID:         entityID,
		ExecutorName:     a.spec.Cognition.Executor.Name,
		ExecutorVersion:  a.spec.Digest,
		ModelPolicy:      a.spec.Cognition.Executor.ModelPolicy,
		PromptVersion:    a.spec.Cognition.Executor.PromptVersion,
		PromptSHA256:     promptDigest,
		ObjectiveSHA256:  objectiveDigest,
		SnapshotSHA256:   evidence.digest,
		AdmissionKey:     admissionKey[:],
		RequestJSON:      requestJSON,
		Traceparent:      traceparent,
		Tracestate:       tracestate,
		CancellationKey:  "episode:" + episodeID,
		SupersessionKey:  "situation:" + item.SituationID,
		DispatchPolicy:   a.spec.Cognition.Executor.DispatchPolicy,
	}
	if _, err := req.WallTimeBudget(); err != nil {
		return nil, fmt.Errorf("validate episode budget: %w", err)
	}
	return req, nil
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

// Persist saves the episode request to the episodes table and marks the
// scheduler item as admitted. It runs inside the supplied transaction. The
// scheduler item must still be pending; otherwise Persist returns an error.
func (a *Assembler) Persist(ctx context.Context, tx *sql.Tx, req *Request, now time.Time) error {
	// P8: an empty dispatch policy is SHADOW — nothing enters action
	// governance unless the spec declared active. The CHECK column stays
	// strict (active|shadow); this is the only place a value is written.
	dispatchPolicy := req.DispatchPolicy
	if dispatchPolicy == "" {
		dispatchPolicy = "shadow"
	}
	snapshotHash, err := canonicaljson.DecodeDigest(req.SnapshotSHA256)
	if err != nil {
		return fmt.Errorf("decode snapshot digest: %w", err)
	}
	promptHash, err := canonicaljson.DecodeDigest(req.PromptSHA256)
	if err != nil {
		return fmt.Errorf("decode prompt digest: %w", err)
	}
	objectiveHash, err := canonicaljson.DecodeDigest(req.ObjectiveSHA256)
	if err != nil {
		return fmt.Errorf("decode objective digest: %w", err)
	}
	if a.cost != nil {
		var payload struct {
			Budget struct {
				CostMicrounits uint64 `json:"cost_microunits"`
			} `json:"budget"`
		}
		if err := json.Unmarshal(req.RequestJSON, &payload); err != nil {
			return fmt.Errorf("decode episode cost budget: %w", err)
		}
		if err := a.cost.Reserve(ctx, tx, req.EpisodeID, req.TenantID, payload.Budget.CostMicrounits, now.UTC().Format(time.RFC3339Nano)); err != nil {
			return fmt.Errorf("reserve episode cost: %w", err)
		}
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO episodes (
			episode_id, scheduler_item_id, tenant_id, situation_id, situation_version,
			executor_name, executor_version, model_policy, prompt_version,
			snapshot_sha256, prompt_sha256, objective_sha256, admission_key, request_json, lifecycle_status, accepted_at,
			dispatch_policy, policy_epoch
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 'admitted', ?, ?, ?)`,
		req.EpisodeID, req.SchedulerItemID, req.TenantID, req.SituationID, req.SituationVersion,
		req.ExecutorName, req.ExecutorVersion, req.ModelPolicy, req.PromptVersion,
		snapshotHash, promptHash, objectiveHash, req.AdmissionKey, req.RequestJSON,
		formatAcceptedAt(now),
		dispatchPolicy, req.PolicyEpoch,
	); err != nil {
		if req.Kind == "reconsider" && isLiveEpisodeConstraint(err) {
			return fmt.Errorf("insert episode: %w: %w", ErrLiveEpisodeConflict, err)
		}
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

func isLiveEpisodeConstraint(err error) bool {
	var sqliteErr *sqlite.Error
	if !errors.As(err, &sqliteErr) || sqliteErr.Code() != sqlite3.SQLITE_CONSTRAINT_UNIQUE {
		return false
	}
	return strings.Contains(err.Error(), "UNIQUE constraint failed: episodes.situation_id")
}

// Rebind rebuilds an admitted episode's request for the live situation version
// (ISSUE-061). The situation advanced past the version the episode was admitted
// under before dispatch; instead of abandoning, the request is re-pointed at the
// live snapshot so the episode reasons over the freshest state. Only snapshot,
// situation_version and snapshot_digest change: the trigger evidence (delta),
// the reconsideration document, identities and trace context are preserved.
// The live snapshot is validated (schema, identity, entity, persisted digest)
// before it can reach a worker; a validation failure returns an error and the
// caller quarantines the episode — a stale snapshot must never reach a worker.
func (a *Assembler) Rebind(ctx context.Context, tx *sql.Tx, req *Request, liveVersion int) (*Request, error) {
	evidence, err := a.loadValidatedSnapshot(ctx, tx, req.SituationID, liveVersion, req.TenantID)
	if err != nil {
		return nil, err
	}
	if evidence.entityID != req.EntityID {
		return nil, fmt.Errorf("live snapshot entity %q does not match bound entity %q", evidence.entityID, req.EntityID)
	}
	var request map[string]any
	if err := json.Unmarshal(req.RequestJSON, &request); err != nil {
		return nil, fmt.Errorf("decode bound episode request: %w", err)
	}
	request["snapshot"] = evidence.document
	request["situation_version"] = liveVersion
	request["snapshot_digest"] = evidence.digest
	requestJSON, err := canonicaljson.Marshal(request)
	if err != nil {
		return nil, fmt.Errorf("marshal re-bound request: %w", err)
	}
	fresh := *req
	fresh.SituationVersion = liveVersion
	fresh.EntityID = evidence.entityID
	fresh.SnapshotSHA256 = evidence.digest
	fresh.RequestJSON = requestJSON
	return &fresh, nil
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

func (a *Assembler) buildTools() []map[string]any {
	tools := make([]map[string]any, 0, len(a.spec.Cognition.Executor.Tools))
	for _, name := range a.spec.Cognition.Executor.Tools {
		tools = append(tools, map[string]any{
			"name":        name,
			"description": "bounded read-only evidence capability",
			"schema":      map[string]any{"type": "object", "additionalProperties": false},
		})
	}
	return tools
}

func (a *Assembler) allowedIntentTypes() map[string]bool {
	configured := make(map[string]bool)
	for _, intent := range a.spec.Actions.Intents {
		configured[intent.Type] = true
	}
	return configured
}

func (a *Assembler) allowedIntentTypeList() []string {
	allowed := a.allowedIntentTypes()
	result := make([]string, 0, len(allowed))
	seen := make(map[string]struct{}, len(allowed))
	for _, intent := range a.spec.Actions.Intents {
		if allowed[intent.Type] {
			if _, ok := seen[intent.Type]; ok {
				continue
			}
			seen[intent.Type] = struct{}{}
			result = append(result, intent.Type)
		}
	}
	return result
}

func (a *Assembler) effectiveRiskCeiling() string {
	ceiling := a.spec.Cognition.Executor.RiskCeiling
	if ceiling == "" {
		return "R1"
	}
	return ceiling
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

type reconsiderationRow struct {
	ReconsiderationID    string
	SituationID          string
	SupersededVersion    int
	CorrectionVersion    int
	InvalidatedCommandID string
	InvalidatedOutcomeID string
	PriorDecisionID      string
	PriorDecisionJSON    []byte
	CommandJSON          []byte
	CommandStatus        string
	IntentID             string
	IntentType           string
	RiskClass            string
	OutcomeID            string
	OutcomeOrdinal       int
	OutcomeStatus        string
	ProviderResultJSON   []byte
	ObservedEffectJSON   []byte
	ReconciliationStatus sql.NullString
	OutcomeSHA256        []byte
}

func loadReconsideration(ctx context.Context, tx *sql.Tx, item schedulerItem, ev evaluation, delta, snapshot map[string]any) (map[string]any, error) {
	supersededVersion := snapshotInt(delta, "superseded_version")
	invalidatedCommandID := snapshotString(delta, "invalidated_command_id")
	var row reconsiderationRow
	if err := tx.QueryRowContext(ctx, `
		SELECT r.reconsideration_id, r.situation_id, r.superseded_version, r.correction_version,
		       r.invalidated_command_id, r.invalidated_outcome_id,
		       d.decision_id, d.raw_json, c.command_json, c.status, i.intent_id, i.intent_type, i.risk_class,
		       o.outcome_id, o.ordinal, o.status, o.provider_result_json, o.observed_effect_json,
		       o.reconciliation_status, o.outcome_sha256
		FROM reconsiderations r
		JOIN commands c ON c.command_id = r.invalidated_command_id
		JOIN intents i ON i.intent_id = c.intent_id
		JOIN decisions d ON d.decision_id = i.decision_id
		JOIN outcomes o ON o.command_id = c.command_id AND o.outcome_id = r.invalidated_outcome_id
		WHERE r.tenant_id = ? AND r.situation_id = ? AND r.correction_version = ?
		  AND (
				r.scheduler_item_id = ? OR
				r.trigger_id = ? OR
				(r.superseded_version = ? AND r.invalidated_command_id = ?)
		  )
		ORDER BY CASE
			WHEN r.scheduler_item_id = ? THEN 0
			WHEN r.trigger_id = ? THEN 1
			ELSE 2
		END
		LIMIT 1`,
		item.TenantID, item.SituationID, item.SituationVersion,
		item.SchedulerItemID, item.TriggerID, supersededVersion, invalidatedCommandID,
		item.SchedulerItemID, item.TriggerID,
	).Scan(
		&row.ReconsiderationID, &row.SituationID, &row.SupersededVersion, &row.CorrectionVersion,
		&row.InvalidatedCommandID, &row.InvalidatedOutcomeID,
		&row.PriorDecisionID, &row.PriorDecisionJSON, &row.CommandJSON, &row.CommandStatus, &row.IntentID, &row.IntentType, &row.RiskClass,
		&row.OutcomeID, &row.OutcomeOrdinal, &row.OutcomeStatus, &row.ProviderResultJSON, &row.ObservedEffectJSON,
		&row.ReconciliationStatus, &row.OutcomeSHA256,
	); err != nil {
		return nil, fmt.Errorf("query reconsideration evidence: %w", err)
	}

	priorDecision, err := jsonDocument(row.PriorDecisionJSON, "prior decision")
	if err != nil {
		return nil, err
	}
	if err := setDocumentIdentity(priorDecision, "decision_id", row.PriorDecisionID); err != nil {
		return nil, err
	}
	command, err := jsonDocument(row.CommandJSON, "executed command")
	if err != nil {
		return nil, err
	}
	if err := setDocumentIdentity(command, "command_id", row.InvalidatedCommandID); err != nil {
		return nil, err
	}
	if err := setDocumentIdentity(command, "intent_id", row.IntentID); err != nil {
		return nil, err
	}
	command["status"] = row.CommandStatus
	command["intent_type"] = row.IntentType
	command["risk_class"] = row.RiskClass
	if _, ok := command["parameters"]; !ok {
		if payload, ok := command["payload"]; ok {
			command["parameters"] = payload
		}
	}

	outcome := map[string]any{
		"outcome_id":            row.OutcomeID,
		"command_id":            row.InvalidatedCommandID,
		"ordinal":               row.OutcomeOrdinal,
		"status":                row.OutcomeStatus,
		"outcome_sha256":        "sha256:" + hex.EncodeToString(row.OutcomeSHA256),
		"reconciliation_status": row.ReconciliationStatus.String,
	}
	if provider, err := optionalJSONDocument(row.ProviderResultJSON, "provider result"); err != nil {
		return nil, err
	} else if provider != nil {
		outcome["provider_result"] = provider
	}
	if observed, err := optionalJSONDocument(row.ObservedEffectJSON, "observed effect"); err != nil {
		return nil, err
	} else if observed != nil {
		outcome["observed_effect"] = observed
	}

	correction := copyDocument(snapshot)
	if nested, ok := delta["correction"].(map[string]any); ok {
		correction = copyDocument(nested)
	}
	correction["reason"] = ev.TriggerName
	correction["invalidates"] = []string{row.InvalidatedCommandID}
	correction["superseded_version"] = row.SupersededVersion
	correction["correction_version"] = row.CorrectionVersion

	return map[string]any{
		"reconsideration_id":     row.ReconsiderationID,
		"situation_id":           row.SituationID,
		"superseded_version":     row.SupersededVersion,
		"correction_version":     row.CorrectionVersion,
		"invalidated_command_id": row.InvalidatedCommandID,
		"invalidated_outcome_id": row.InvalidatedOutcomeID,
		"prior_decision":         priorDecision,
		"commands":               []map[string]any{command},
		"outcomes":               []map[string]any{outcome},
		"correction":             correction,
	}, nil
}

func jsonDocument(raw []byte, name string) (map[string]any, error) {
	var document map[string]any
	if len(raw) == 0 {
		return nil, fmt.Errorf("%s is empty", name)
	}
	if err := json.Unmarshal(raw, &document); err != nil {
		return nil, fmt.Errorf("decode %s: %w", name, err)
	}
	if document == nil {
		return nil, fmt.Errorf("%s must be an object", name)
	}
	return document, nil
}

func optionalJSONDocument(raw []byte, name string) (any, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	var document any
	if err := json.Unmarshal(raw, &document); err != nil {
		return nil, fmt.Errorf("decode %s: %w", name, err)
	}
	return document, nil
}

func setDocumentIdentity(document map[string]any, key, want string) error {
	if want == "" {
		return fmt.Errorf("%s is required", key)
	}
	if got, ok := document[key]; ok && got != want {
		return fmt.Errorf("%s identity mismatch: got %v, want %s", key, got, want)
	}
	document[key] = want
	return nil
}

func copyDocument(document map[string]any) map[string]any {
	copy := make(map[string]any, len(document))
	for key, value := range document {
		copy[key] = value
	}
	return copy
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
