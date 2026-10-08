package domain

import (
	"bytes"
	"encoding/json"
	"fmt"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/canonicaljson"
	"github.com/ghassan-ai-projects/agentic-stream/internal/contractsv1"
	"github.com/ghassan-ai-projects/agentic-stream/internal/decisions"
)

type ShadowRules struct {
	Catalog      *decisions.IntentCatalog
	AllowedTypes map[string]struct{}
	RiskCeiling  string
	PolicyDigest string
}

type ValidatedOutput struct {
	Output      ShadowOutput
	Canonical   []byte
	Decision    *decisions.Result
	DecisionSHA []byte
	ManifestSHA []byte
}

func (r ShadowRules) ValidateOutput(input ShadowInput, output ShadowOutput, now time.Time) (ValidatedOutput, error) {
	bound, err := r.bindOutput(output)
	if err != nil {
		return ValidatedOutput{}, err
	}
	entityID, err := ShadowEntityID(input.SnapshotJSON)
	if err != nil {
		return ValidatedOutput{}, err
	}
	return r.validateDecision(input, bound, entityID, now)
}

func (r ShadowRules) bindOutput(output ShadowOutput) (ValidatedOutput, error) {
	manifestSHA, err := bindShadowManifest(output)
	if err != nil {
		return ValidatedOutput{}, err
	}
	canonical, document, err := canonicalShadowDecision(output)
	if err != nil {
		return ValidatedOutput{}, err
	}
	decisionSHA, err := bindShadowDecisionDigest(output, document)
	if err != nil {
		return ValidatedOutput{}, err
	}
	return ValidatedOutput{Output: output, Canonical: canonical, DecisionSHA: decisionSHA, ManifestSHA: manifestSHA}, nil
}

func bindShadowManifest(output ShadowOutput) ([]byte, error) {
	if output.ExecutorVersion == "" {
		return nil, fmt.Errorf("executor version is required")
	}
	manifestSHA, err := canonicaljson.DecodeDigest(output.ManifestSHA256)
	if err != nil {
		return nil, fmt.Errorf("manifest digest: %w", err)
	}
	return manifestSHA, nil
}

func canonicalShadowDecision(output ShadowOutput) ([]byte, map[string]any, error) {
	document, err := contractsv1.DecodeDocumentJSON(output.DecisionJSON)
	if err != nil {
		return nil, nil, fmt.Errorf("decision JSON: %w", err)
	}
	canonical, err := canonicaljson.Marshal(document)
	if err != nil || !bytes.Equal(canonical, output.DecisionJSON) {
		return nil, nil, fmt.Errorf("decision JSON is not canonical")
	}
	return canonical, document, nil
}

func bindShadowDecisionDigest(output ShadowOutput, document map[string]any) ([]byte, error) {
	decisionSHA, err := canonicaljson.DecodeDigest(output.DecisionSHA256)
	if err != nil {
		return nil, fmt.Errorf("decision digest: %w", err)
	}
	if !contractsv1.VerifyDocumentDigest(canonicaljson.DomainDecision, document, decisionSHA) {
		return nil, fmt.Errorf("decision digest does not match decision JSON")
	}
	return decisionSHA, nil
}

func (r ShadowRules) validateDecision(input ShadowInput, bound ValidatedOutput, entityID string, now time.Time) (ValidatedOutput, error) {
	validated, err := decisions.Validate(bound.Canonical, bound.Output.DecisionSHA256, decisions.Input{
		EpisodeID: input.EpisodeID, AttemptID: input.AttemptID, Fence: input.Fence,
		TenantID: input.TenantID, SituationID: input.SituationID,
		SituationVersion: input.SituationVersion, EntityID: entityID, SnapshotDigest: input.SnapshotDigest,
		AllowedIntentTypes: r.AllowedTypes, RiskCeiling: r.RiskCeiling, IntentCatalog: r.Catalog,
		Now: now,
	})
	if err != nil {
		return ValidatedOutput{}, fmt.Errorf("validate shadow decision: %w", err)
	}
	bound.Decision = validated
	return bound, nil
}

func ShadowEntityID(snapshot []byte) (string, error) {
	var document struct {
		Entity struct {
			ID string `json:"id"`
		} `json:"entity"`
	}
	if err := json.Unmarshal(snapshot, &document); err != nil {
		return "", fmt.Errorf("decode shadow entity: %w", err)
	}
	if document.Entity.ID == "" {
		return "", fmt.Errorf("shadow snapshot entity id is required")
	}
	return document.Entity.ID, nil
}
