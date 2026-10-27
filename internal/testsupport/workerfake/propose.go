package workerfake

import (
	"cmp"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/ghassan-ai-projects/agentic-stream/internal/canonicaljson"
	"github.com/ghassan-ai-projects/agentic-stream/internal/contractsv1"
	"github.com/ghassan-ai-projects/agentic-stream/internal/kernel"
	runtimev1 "github.com/ghassan-ai-projects/agentic-stream/proto/agenticstream/runtime/v1"
)

var proposalUsage = &runtimev1.Usage{InputTokens: 1200, OutputTokens: 300}

var errSnapshotWithoutEntity = errors.New("snapshot has no entity id")

// Answer is what a fake worker decides for one episode: a hypothesis, an
// optional diagnosis code, its intents and the usage it reports (nil for a
// fixed one).
type Answer struct {
	Hypothesis    string
	DiagnosisCode string
	Summary       string
	Intents       []ProposedIntent
	Usage         *runtimev1.Usage
}

// ProposedIntent is one intent of an Answer: its type, risk class and the
// model-writable parameters.
type ProposedIntent struct {
	Type       string         `json:"type"`
	Risk       string         `json:"risk"`
	Parameters map[string]any `json:"parameters"`
}

// Propose streams a produced decision for the answer the way Tamoz's
// EpisodeStream does: a budget update, the decision and a terminal event whose
// usage proves the attempt stayed inside its limits.
func Propose(req *runtimev1.EpisodeRequest, emit func(*runtimev1.EpisodeEvent) error, answer Answer) error {
	decisionJSON, digest, err := decisionFor(req, answer)
	if err != nil {
		return err
	}
	usage := cmp.Or(answer.Usage, proposalUsage)
	if err := emit(episodeEvent(req, 2, &runtimev1.EpisodeEvent_Budget{Budget: &runtimev1.BudgetUpdated{CumulativeUsage: usage, ModelCallsUsed: 1}})); err != nil {
		return err
	}
	if err := emit(decisionEvent(req, decisionJSON, digest)); err != nil {
		return err
	}
	return emit(episodeEvent(req, 4, &runtimev1.EpisodeEvent_Terminal{Terminal: &runtimev1.Terminal{
		Status: runtimev1.TerminalStatus_TERMINAL_STATUS_PRODUCED, ReasonCode: "decision_produced", Usage: usage,
	}}))
}

// Fail ends the episode the way a worker does when it produces no decision: a
// failed terminal event with its reason and usage.
func Fail(req *runtimev1.EpisodeRequest, emit func(*runtimev1.EpisodeEvent) error, reason string, usage *runtimev1.Usage) error {
	return emit(episodeEvent(req, 2, &runtimev1.EpisodeEvent_Terminal{Terminal: &runtimev1.Terminal{
		Status: runtimev1.TerminalStatus_TERMINAL_STATUS_FAILED, ReasonCode: reason, Usage: usage,
	}}))
}

func decisionEvent(req *runtimev1.EpisodeRequest, decisionJSON, digest []byte) *runtimev1.EpisodeEvent {
	return episodeEvent(req, 3, &runtimev1.EpisodeEvent_Decision{Decision: &runtimev1.DecisionProposed{
		DecisionJson: decisionJSON, DecisionSha256: digest, EpisodeId: req.GetEpisodeId(), AttemptId: req.GetAttemptId(), Fence: req.GetFence(),
	}})
}

func decisionFor(req *runtimev1.EpisodeRequest, answer Answer) ([]byte, []byte, error) {
	decision, err := decisionDocument(req, answer)
	if err != nil {
		return nil, nil, err
	}
	return canonicalDecision(decision)
}

func canonicalDecision(decision map[string]any) ([]byte, []byte, error) {
	decisionJSON, err := canonicaljson.Marshal(decision)
	if err != nil {
		return nil, nil, fmt.Errorf("marshal decision: %w", err)
	}
	digest, err := canonicaljson.Digest(canonicaljson.DomainDecision, decision)
	if err != nil {
		return nil, nil, fmt.Errorf("digest decision: %w", err)
	}
	digestBytes, err := kernel.DecodeDigest(digest)
	if err != nil {
		return nil, nil, fmt.Errorf("decode decision digest: %w", err)
	}
	return decisionJSON, digestBytes, nil
}

func decisionDocument(req *runtimev1.EpisodeRequest, answer Answer) (map[string]any, error) {
	scope := fmt.Sprintf("%s.%s.%d", req.GetEpisodeId(), req.GetAttemptId(), req.GetFence())
	validUntil := time.Now().UTC().Add(10 * time.Minute).Format(time.RFC3339)
	intents, err := answerIntents(req, answer, scope, validUntil)
	if err != nil {
		return nil, err
	}
	decision := decisionBody(req, answer, scope, validUntil)
	decision["intents"] = intents
	if len(answer.Intents) == 0 {
		decision["decision_type"] = "need_more_evidence"
	}
	if answer.DiagnosisCode != "" {
		decision["diagnosis_code"] = answer.DiagnosisCode
	}
	return decision, nil
}

func decisionBody(req *runtimev1.EpisodeRequest, answer Answer, scope, validUntil string) map[string]any {
	return map[string]any{
		"decision_id": "decision." + scope, "episode_id": req.GetEpisodeId(), "attempt_id": req.GetAttemptId(), "fence": req.GetFence(),
		"snapshot_digest": kernel.EncodeDigest(req.GetSnapshotSha256()),
		"situation_id":    req.GetSituationId(), "situation_version": req.GetSituationVersion(),
		"primary_hypothesis": answer.Hypothesis, "confidence": 0.9, "summary": answer.Summary,
		"facts_used": []any{}, "alternatives": []any{}, "valid_until": validUntil,
	}
}

func answerIntents(req *runtimev1.EpisodeRequest, answer Answer, scope, validUntil string) ([]any, error) {
	intents := []any{}
	if len(answer.Intents) == 0 {
		return intents, nil
	}
	entityID, err := SnapshotEntityID(req.GetSnapshotJson())
	if err != nil {
		return nil, err
	}
	for _, proposed := range answer.Intents {
		intent, err := intentDocument(req, proposed, scope, entityID, validUntil)
		if err != nil {
			return nil, err
		}
		intents = append(intents, intent)
	}
	return intents, nil
}

func intentDocument(req *runtimev1.EpisodeRequest, proposed ProposedIntent, scope, entityID, validUntil string) (map[string]any, error) {
	parameters := map[string]any{"entity_id": entityID, "situation_id": req.GetSituationId(), "situation_version": req.GetSituationVersion()}
	maps.Copy(parameters, proposed.Parameters)
	intent := map[string]any{
		"intent_id": "intent." + scope + "." + proposed.Type, "decision_id": "decision." + scope,
		"tenant_id": req.GetTenantId(), "situation_id": req.GetSituationId(), "situation_version": req.GetSituationVersion(),
		"type": proposed.Type, "risk_class": proposed.Risk, "evidence_ids": []any{}, "expires_at": validUntil, "parameters": parameters,
	}
	intentDigest, err := contractsv1.IntentDigest(intent)
	if err != nil {
		return nil, fmt.Errorf("digest intent: %w", err)
	}
	intent["intent_digest"] = intentDigest
	return intent, nil
}

// SnapshotEntityID reads the entity id of an episode snapshot.
func SnapshotEntityID(snapshotJSON []byte) (string, error) {
	var snapshot struct {
		Entity struct {
			ID string `json:"id"`
		} `json:"entity"`
	}
	if err := json.Unmarshal(snapshotJSON, &snapshot); err != nil {
		return "", fmt.Errorf("decode the snapshot: %w", err)
	}
	if snapshot.Entity.ID == "" {
		return "", errSnapshotWithoutEntity
	}
	return snapshot.Entity.ID, nil
}

func episodeEvent(req *runtimev1.EpisodeRequest, sequence uint64, payload any) *runtimev1.EpisodeEvent {
	event := &runtimev1.EpisodeEvent{EpisodeId: req.GetEpisodeId(), Sequence: sequence, AttemptId: req.GetAttemptId(), Fence: req.GetFence(), OccurredAt: timestamppb.Now()}
	switch p := payload.(type) {
	case *runtimev1.EpisodeEvent_Budget:
		event.Payload = p
	case *runtimev1.EpisodeEvent_Decision:
		event.Payload = p
	case *runtimev1.EpisodeEvent_Terminal:
		event.Payload = p
	}
	return event
}
