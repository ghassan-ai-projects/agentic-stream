package rule

import (
	"encoding/json"
	"fmt"
	"maps"
	"slices"
	"strconv"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/contractsv1"
	"github.com/ghassan-ai-projects/agentic-stream/internal/episodes"
	"github.com/ghassan-ai-projects/agentic-stream/internal/kernel"
	"github.com/ghassan-ai-projects/agentic-stream/internal/sources"
)

type recommendation struct {
	RuleID         string         `json:"rule_id"`
	RuleVersion    int            `json:"rule_version"`
	PlaybookDigest string         `json:"playbook_digest"`
	DiagnosisCode  string         `json:"diagnosis_code"`
	Intent         string         `json:"intent"`
	Preset         string         `json:"preset"`
	Parameters     map[string]any `json:"parameters"`
	Features       map[string]any `json:"features"`
}

type catalogEntry struct {
	Type            string `json:"type"`
	RiskClass       string `json:"risk_class"`
	ParameterSchema struct {
		Properties map[string]any `json:"properties"`
	} `json:"parameter_schema"`
}

type boundRequest struct {
	Rule     recommendation `json:"rule"`
	Executor struct {
		IntentCatalog []catalogEntry `json:"intent_catalog"`
	} `json:"executor"`
}

func readRequest(req *episodes.Request) (boundRequest, error) {
	var bound boundRequest
	if err := json.Unmarshal(req.RequestJSON, &bound); err != nil {
		return boundRequest{}, fmt.Errorf("decode the rule request of episode %s: %w", req.EpisodeID, err)
	}
	if bound.Rule.RuleID == "" || bound.Rule.Intent == "" {
		return boundRequest{}, fmt.Errorf("the request of episode %s carries no rule", req.EpisodeID)
	}
	return bound, nil
}

func (b boundRequest) entry() (catalogEntry, error) {
	for _, entry := range b.Executor.IntentCatalog {
		if entry.Type == b.Rule.Intent {
			return entry, nil
		}
	}
	return catalogEntry{}, fmt.Errorf("rule %s recommends %s, which the intent catalog does not declare", b.Rule.RuleID, b.Rule.Intent)
}

func (b boundRequest) intent(req *episodes.Request, expiresAt time.Time) (map[string]any, error) {
	entry, err := b.entry()
	if err != nil {
		return nil, err
	}
	intent := b.unsealedIntent(req, entry, expiresAt)
	digest, err := contractsv1.IntentDigest(intent)
	if err != nil {
		return nil, fmt.Errorf("digest the rule intent of episode %s: %w", req.EpisodeID, err)
	}
	intent["intent_digest"] = digest
	return intent, nil
}

func (b boundRequest) unsealedIntent(req *episodes.Request, entry catalogEntry, expiresAt time.Time) map[string]any {
	return map[string]any{
		"intent_id": sources.PrefixIntent + req.EpisodeID, "decision_id": sources.PrefixDecision + req.EpisodeID,
		"tenant_id": req.TenantID, "situation_id": req.SituationID, "situation_version": req.SituationVersion,
		"type": entry.Type, "risk_class": entry.RiskClass, "parameters": b.parameters(req, entry),
		"expires_at": kernel.FormatTime(expiresAt),
	}
}

func (b boundRequest) parameters(req *episodes.Request, entry catalogEntry) map[string]any {
	parameters := maps.Clone(b.Rule.Parameters)
	if parameters == nil {
		parameters = map[string]any{}
	}
	bindings := map[string]any{"entity_id": req.EntityID, "situation_id": req.SituationID, "situation_version": req.SituationVersion}
	for name, value := range bindings {
		if _, declared := entry.ParameterSchema.Properties[name]; declared {
			parameters[name] = value
		}
	}
	return parameters
}

func (b boundRequest) decision(req *episodes.Request, intent map[string]any) map[string]any {
	return map[string]any{
		"decision_id": sources.PrefixDecision + req.EpisodeID, "episode_id": req.EpisodeID, "attempt_id": req.AttemptID,
		"fence": req.Fence, "snapshot_digest": req.SnapshotSHA256, "situation_id": req.SituationID,
		"situation_version": req.SituationVersion, "diagnosis_code": b.Rule.DiagnosisCode, "confidence": 1.0,
		"summary": b.summary(), "facts_used": b.facts(), "intents": []map[string]any{intent},
	}
}

func (b boundRequest) facts() []map[string]any {
	facts := make([]map[string]any, 0, len(b.Rule.Features))
	for _, name := range slices.Sorted(maps.Keys(b.Rule.Features)) {
		facts = append(facts, map[string]any{"path": "features." + name, "value": b.Rule.Features[name]})
	}
	return facts
}

func (b boundRequest) summary() string {
	return "rule " + b.Rule.RuleID + " v" + strconv.Itoa(b.Rule.RuleVersion) + " of playbook " + b.Rule.PlaybookDigest +
		": " + b.Rule.DiagnosisCode + " -> " + b.Rule.Intent + "/" + b.Rule.Preset
}
