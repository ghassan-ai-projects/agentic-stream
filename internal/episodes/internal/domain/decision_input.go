package domain

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/canonicaljson"
	"github.com/ghassan-ai-projects/agentic-stream/internal/decisions"
	"github.com/ghassan-ai-projects/agentic-stream/internal/episodeledger"
	"github.com/ghassan-ai-projects/agentic-stream/internal/spec"
)

type decisionRequestAuthority struct {
	AllowedIntentTypes []string `json:"allowed_intent_types"`
	RiskCeiling        string   `json:"risk_ceiling"`
	Kind               string   `json:"kind"`
	Executor           struct {
		IntentCatalog       []map[string]any `json:"intent_catalog"`
		IntentCatalogSHA256 string           `json:"intent_catalog_sha256"`
		DiagnosisCatalog    string           `json:"diagnosis_catalog"`
	} `json:"executor"`
}

func DecisionInput(req *Request, identity episodeledger.Identity, now time.Time) (decisions.Input, error) {
	var payload decisionRequestAuthority
	if err := json.Unmarshal(req.RequestJSON, &payload); err != nil {
		return decisions.Input{}, fmt.Errorf("decode request tools: %w", err)
	}
	return payload.validationInput(req, identity, now)
}

func (payload decisionRequestAuthority) validationInput(req *Request, identity episodeledger.Identity, now time.Time) (decisions.Input, error) {
	allowed := make(map[string]struct{}, len(payload.AllowedIntentTypes))
	for _, intentType := range payload.AllowedIntentTypes {
		allowed[intentType] = struct{}{}
	}
	if payload.RiskCeiling == "" {
		return decisions.Input{}, fmt.Errorf("request has no explicit risk ceiling")
	}
	compiled, err := payload.verifiedIntentCatalog()
	if err != nil {
		return decisions.Input{}, err
	}
	input := payload.boundValidationInput(req, identity, now, allowed, compiled)
	input.DiagnosisCodes, err = payload.diagnosisCodes()
	return input, err
}

func (payload decisionRequestAuthority) diagnosisCodes() (map[string]struct{}, error) {
	if payload.Executor.DiagnosisCatalog == "" {
		return nil, nil
	}
	entries, err := spec.ParseDiagnosisCodes(payload.Executor.DiagnosisCatalog)
	if err != nil {
		return nil, fmt.Errorf("read diagnosis catalog of the request: %w", err)
	}
	codes := make(map[string]struct{}, len(entries))
	for _, code := range entries {
		codes[code] = struct{}{}
	}
	return codes, nil
}

func (payload decisionRequestAuthority) verifiedIntentCatalog() (*decisions.IntentCatalog, error) {
	if !canonicaljson.Verify(canonicaljson.DomainIntentCatalog, payload.Executor.IntentCatalog, payload.Executor.IntentCatalogSHA256) {
		return nil, fmt.Errorf("intent catalog is missing, forged, or malformed")
	}
	compiled, err := decisions.CompileIntentCatalog(payload.Executor.IntentCatalog)
	if err != nil {
		return nil, fmt.Errorf("compile intent catalog: %w", err)
	}
	return compiled, nil
}

func (payload decisionRequestAuthority) boundValidationInput(req *Request, identity episodeledger.Identity, now time.Time, allowed map[string]struct{}, compiled *decisions.IntentCatalog) decisions.Input {
	return decisions.Input{
		EpisodeID: identity.EpisodeID, AttemptID: identity.AttemptID, Fence: identity.Fence,
		TenantID: req.TenantID, SituationID: req.SituationID, SituationVersion: req.SituationVersion,
		EntityID: req.EntityID, SnapshotDigest: req.SnapshotSHA256,
		AllowedIntentTypes: allowed, RiskCeiling: payload.RiskCeiling, IntentCatalog: compiled,
		Reconsider: payload.Kind == episodeledger.KindReconsider, Now: now,
	}
}
