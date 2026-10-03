package replay

import (
	"bytes"
	"encoding/json"
	"fmt"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/canonicaljson"
	"github.com/ghassan-ai-projects/agentic-stream/internal/decisions"
)

type validatedShadowOutput struct {
	output      ShadowOutput
	canonical   []byte
	decision    *decisions.Result
	decisionSHA []byte
	manifestSHA []byte
}

func validateShadowPair(input ShadowInput, baselineOutput, tamozOutput ShadowOutput, validation shadowValidation, evaluationTime time.Time) (validatedShadowOutput, validatedShadowOutput, error) {
	baseline, err := validateShadowOutput(input, baselineOutput, validation.catalog, validation.allowedTypes, validation.riskCeiling, evaluationTime)
	if err != nil {
		return validatedShadowOutput{}, validatedShadowOutput{}, fmt.Errorf("validate baseline shadow episode %s: %w", input.EpisodeKey, err)
	}
	tamoz, err := validateShadowOutput(input, tamozOutput, validation.catalog, validation.allowedTypes, validation.riskCeiling, evaluationTime)
	if err != nil {
		return validatedShadowOutput{}, validatedShadowOutput{}, fmt.Errorf("validate Tamoz shadow episode %s: %w", input.EpisodeKey, err)
	}
	return baseline, tamoz, nil
}

func validateShadowOutput(input ShadowInput, output ShadowOutput, catalog *decisions.IntentCatalog, allowedTypes map[string]struct{}, riskCeiling string, now time.Time) (validatedShadowOutput, error) {
	bound, err := bindShadowOutput(output)
	if err != nil {
		return validatedShadowOutput{}, err
	}
	entityID, err := shadowEntityID(input.SnapshotJSON)
	if err != nil {
		return validatedShadowOutput{}, err
	}
	if riskCeiling == "" {
		riskCeiling = "R1"
	}
	return validateBoundShadowDecision(input, bound, catalog, allowedTypes, riskCeiling, now, entityID)
}

func bindShadowOutput(output ShadowOutput) (validatedShadowOutput, error) {
	manifestSHA, err := bindShadowManifest(output)
	if err != nil {
		return validatedShadowOutput{}, err
	}
	canonical, document, err := canonicalShadowDecision(output)
	if err != nil {
		return validatedShadowOutput{}, err
	}
	decisionSHA, err := bindShadowDecisionDigest(output, document)
	if err != nil {
		return validatedShadowOutput{}, err
	}
	return validatedShadowOutput{output: output, canonical: canonical, decisionSHA: decisionSHA, manifestSHA: manifestSHA}, nil
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
	canonical, err := canonicaljson.Marshal(json.RawMessage(output.DecisionJSON))
	if err != nil {
		return nil, nil, fmt.Errorf("decision JSON: %w", err)
	}
	if !bytes.Equal(canonical, output.DecisionJSON) {
		return nil, nil, fmt.Errorf("decision JSON is not canonical")
	}
	var document map[string]any
	if err := json.Unmarshal(canonical, &document); err != nil {
		return nil, nil, fmt.Errorf("decode decision JSON: %w", err)
	}
	return canonical, document, nil
}

func bindShadowDecisionDigest(output ShadowOutput, document map[string]any) ([]byte, error) {
	decisionSHA, err := canonicaljson.DecodeDigest(output.DecisionSHA256)
	if err != nil {
		return nil, fmt.Errorf("decision digest: %w", err)
	}
	if !canonicaljson.Verify(canonicaljson.DomainDecision, document, output.DecisionSHA256) {
		return nil, fmt.Errorf("decision digest does not match decision JSON")
	}
	return decisionSHA, nil
}

func validateBoundShadowDecision(input ShadowInput, bound validatedShadowOutput, catalog *decisions.IntentCatalog, allowedTypes map[string]struct{}, riskCeiling string, now time.Time, entityID string) (validatedShadowOutput, error) {
	validated, err := decisions.Validate(bound.canonical, bound.output.DecisionSHA256, decisions.Input{
		EpisodeID: input.EpisodeID, AttemptID: input.AttemptID, Fence: input.Fence,
		TenantID: input.TenantID, SituationID: input.SituationID,
		SituationVersion: input.SituationVersion, EntityID: entityID, SnapshotDigest: input.SnapshotDigest,
		AllowedIntentTypes: allowedTypes, RiskCeiling: riskCeiling, IntentCatalog: catalog,
		Kind: "standard", Now: now,
	})
	if err != nil {
		return validatedShadowOutput{}, fmt.Errorf("validate shadow decision: %w", err)
	}
	bound.decision = validated
	return bound, nil
}
