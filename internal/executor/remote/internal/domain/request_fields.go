package domain

import (
	"encoding/json"
	"fmt"

	"github.com/ghassan-ai-projects/agentic-stream/internal/contractsv1"
	"github.com/ghassan-ai-projects/agentic-stream/internal/episodeledger"
	"github.com/ghassan-ai-projects/agentic-stream/internal/spec"
	runtimev1 "github.com/ghassan-ai-projects/agentic-stream/proto/agenticstream/runtime/v1"
)

type reconsiderationRequest struct {
	PriorDecision json.RawMessage   `json:"prior_decision"`
	Commands      []json.RawMessage `json:"commands"`
	Outcomes      []json.RawMessage `json:"outcomes"`
	Correction    json.RawMessage   `json:"correction"`
}

func dispatchPolicyEnum(policy string) runtimev1.DispatchPolicy {
	if policy == spec.DispatchActive {
		return runtimev1.DispatchPolicy_DISPATCH_POLICY_ACTIVE
	}
	return runtimev1.DispatchPolicy_DISPATCH_POLICY_SHADOW
}

func marshalIntentCatalog(entries []map[string]any) ([]byte, error) {
	if len(entries) == 0 {
		return nil, nil
	}
	encoded, err := json.Marshal(entries)
	if err != nil {
		return nil, fmt.Errorf("marshal intent catalog: %w", err)
	}
	return encoded, nil
}

func reconsiderationMessage(payload *reconsiderationRequest) (*runtimev1.Reconsideration, error) {
	if payload == nil {
		return nil, fmt.Errorf("payload is required")
	}
	priorDecision, err := requiredJSON(payload.PriorDecision, "prior_decision")
	if err != nil {
		return nil, err
	}
	correction, err := requiredJSON(payload.Correction, "correction")
	if err != nil {
		return nil, err
	}
	return reconsiderationEvidence(payload, priorDecision, correction)
}

func reconsiderationEvidence(payload *reconsiderationRequest, priorDecision, correction []byte) (*runtimev1.Reconsideration, error) {
	commands, err := requiredJSONList(payload.Commands, "commands")
	if err != nil {
		return nil, err
	}
	outcomes, err := requiredJSONList(payload.Outcomes, "outcomes")
	if err != nil {
		return nil, err
	}
	return &runtimev1.Reconsideration{
		PriorDecisionJson:   priorDecision,
		ExecutedCommandJson: commands,
		ObservedOutcomeJson: outcomes,
		CorrectionJson:      correction,
	}, nil
}

func requiredJSONList(items []json.RawMessage, name string) ([][]byte, error) {
	encoded := make([][]byte, 0, len(items))
	for index, item := range items {
		value, err := requiredJSON(item, fmt.Sprintf("%s[%d]", name, index))
		if err != nil {
			return nil, err
		}
		encoded = append(encoded, value)
	}
	return encoded, nil
}

func requiredJSON(raw json.RawMessage, name string) ([]byte, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return nil, fmt.Errorf("%s is required", name)
	}
	return append([]byte(nil), raw...), nil
}

func episodeKind(value string) (runtimev1.EpisodeKind, error) {
	switch value {
	case episodeledger.KindStandard:
		return runtimev1.EpisodeKind_EPISODE_KIND_DIAGNOSE, nil
	case episodeledger.KindReconsider:
		return runtimev1.EpisodeKind_EPISODE_KIND_RECONSIDER, nil
	default:
		return 0, fmt.Errorf("unsupported episode kind %q", value)
	}
}

func episodeLane(value string) (runtimev1.EpisodeLane, error) {
	switch value {
	case spec.LaneFast:
		return runtimev1.EpisodeLane_EPISODE_LANE_FAST, nil
	case spec.LaneDeep:
		return runtimev1.EpisodeLane_EPISODE_LANE_DEEP, nil
	default:
		return 0, fmt.Errorf("unsupported episode lane %q", value)
	}
}

func riskClass(value string) (runtimev1.RiskClass, error) {
	risk := contractsv1.RiskClass(value)
	if !risk.Valid() {
		return 0, fmt.Errorf("unsupported risk ceiling %q", value)
	}
	return runtimev1.RiskClass(runtimev1.RiskClass_value["RISK_CLASS_"+string(risk)]), nil
}
