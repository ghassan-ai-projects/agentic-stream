package replay

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/qualification"

	"github.com/ghassan-ai-projects/agentic-stream/internal/canonicaljson"
)

type builtShadowComparison struct {
	record qualification.ShadowComparison
	result ShadowComparisonResult
}

func buildShadowComparison(input ShadowInput, baseline, tamoz validatedShadowOutput, tenantID string, createdAt time.Time) (builtShadowComparison, error) {
	comparisonKey := tenantID + ":" + input.EpisodeKey
	document := shadowComparisonDocument(input, baseline, tamoz, tenantID, comparisonKey)
	comparisonJSON, err := canonicaljson.Marshal(document)
	if err != nil {
		return builtShadowComparison{}, fmt.Errorf("canonicalize comparison: %w", err)
	}
	comparisonDigest, err := canonicaljson.Digest(canonicaljson.DomainShadowComparison, document)
	if err != nil {
		return builtShadowComparison{}, fmt.Errorf("digest comparison: %w", err)
	}
	comparisonSHA, err := canonicaljson.DecodeDigest(comparisonDigest)
	if err != nil {
		return builtShadowComparison{}, fmt.Errorf("decode comparison digest: %w", err)
	}
	snapshotSHA, err := canonicaljson.DecodeDigest(input.SnapshotDigest)
	if err != nil {
		return builtShadowComparison{}, fmt.Errorf("decode snapshot digest: %w", err)
	}
	specSHA, err := canonicaljson.DecodeDigest(input.SpecDigest)
	if err != nil {
		return builtShadowComparison{}, fmt.Errorf("decode spec digest: %w", err)
	}
	policySHA, err := canonicaljson.DecodeDigest(input.PolicyDigest)
	if err != nil {
		return builtShadowComparison{}, fmt.Errorf("decode policy digest: %w", err)
	}
	comparisonID := "cmp_" + hex.EncodeToString(comparisonSHA)
	return builtShadowComparison{
		record: qualification.ShadowComparison{
			ComparisonID: comparisonID, ComparisonKey: comparisonKey, TenantID: tenantID,
			EpisodeID: input.EpisodeID, SituationID: input.SituationID, SituationVersion: input.SituationVersion,
			TriggerID: input.TriggerID, SnapshotSHA256: snapshotSHA, SpecSHA256: specSHA, PolicySHA256: policySHA,
			BaselineExecutorVersion: baseline.output.ExecutorVersion, TamozExecutorVersion: tamoz.output.ExecutorVersion,
			BaselineManifestSHA256: baseline.manifestSHA, TamozManifestSHA256: tamoz.manifestSHA,
			BaselineDecisionJSON: baseline.canonical, BaselineDecisionSHA256: baseline.decisionSHA,
			TamozDecisionJSON: tamoz.canonical, TamozDecisionSHA256: tamoz.decisionSHA,
			ComparisonJSON: comparisonJSON, ComparisonSHA256: comparisonSHA, CreatedAt: createdAt.UTC().Format(time.RFC3339Nano),
		},
		result: ShadowComparisonResult{EpisodeKey: input.EpisodeKey, ComparisonSHA256: comparisonDigest,
			BaselineDecisionSHA256: baseline.output.DecisionSHA256, TamozDecisionSHA256: tamoz.output.DecisionSHA256,
			DecisionsEqual: bytes.Equal(baseline.canonical, tamoz.canonical)},
	}, nil
}

func shadowComparisonDocument(input ShadowInput, baseline, tamoz validatedShadowOutput, tenantID, comparisonKey string) map[string]any {
	differences := make([]string, 0, 2)
	if !bytes.Equal(baseline.canonical, tamoz.canonical) {
		differences = append(differences, "decision")
	}
	if baseline.output.ManifestSHA256 != tamoz.output.ManifestSHA256 {
		differences = append(differences, "manifest")
	}
	return map[string]any{
		"comparison_key":    comparisonKey,
		"tenant_id":         tenantID,
		"episode_id":        input.EpisodeID,
		"situation_id":      input.SituationID,
		"situation_version": input.SituationVersion,
		"trigger_id":        input.TriggerID,
		"snapshot_digest":   input.SnapshotDigest,
		"spec_digest":       input.SpecDigest,
		"policy_digest":     input.PolicyDigest,
		"baseline": map[string]any{
			"executor_version": baseline.output.ExecutorVersion,
			"manifest_sha256":  baseline.output.ManifestSHA256,
			"decision_sha256":  baseline.output.DecisionSHA256,
		},
		"tamoz": map[string]any{
			"executor_version": tamoz.output.ExecutorVersion,
			"manifest_sha256":  tamoz.output.ManifestSHA256,
			"decision_sha256":  tamoz.output.DecisionSHA256,
		},
		"differences": differences,
	}
}
