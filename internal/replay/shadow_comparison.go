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

type shadowDigests struct{ snapshot, spec, policy []byte }

func buildShadowComparison(input ShadowInput, baseline, tamoz validatedShadowOutput, tenantID string, createdAt time.Time) (builtShadowComparison, error) {
	comparisonKey := tenantID + ":" + input.EpisodeKey
	document := shadowComparisonDocument(input, baseline, tamoz, tenantID, comparisonKey)
	comparisonJSON, comparisonDigest, comparisonSHA, err := sealShadowComparison(document)
	if err != nil {
		return builtShadowComparison{}, err
	}
	digests, err := shadowInputDigests(input)
	if err != nil {
		return builtShadowComparison{}, err
	}
	comparisonID := "cmp_" + hex.EncodeToString(comparisonSHA)
	return assembleShadowComparison(input, baseline, tamoz, tenantID, comparisonKey, comparisonID, comparisonJSON, comparisonDigest, comparisonSHA, digests, createdAt), nil
}

func shadowComparisonDocument(input ShadowInput, baseline, tamoz validatedShadowOutput, tenantID, comparisonKey string) map[string]any {
	differences := shadowDifferences(baseline, tamoz)
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
		"baseline":          shadowOutputProvenance(baseline),
		"tamoz":             shadowOutputProvenance(tamoz),
		"differences":       differences,
	}
}

func shadowDifferences(baseline, tamoz validatedShadowOutput) []string {
	differences := make([]string, 0, 2)
	if !bytes.Equal(baseline.canonical, tamoz.canonical) {
		differences = append(differences, "decision")
	}
	if baseline.output.ManifestSHA256 != tamoz.output.ManifestSHA256 {
		differences = append(differences, "manifest")
	}
	return differences
}

func shadowOutputProvenance(value validatedShadowOutput) map[string]any {
	return map[string]any{"executor_version": value.output.ExecutorVersion, "manifest_sha256": value.output.ManifestSHA256, "decision_sha256": value.output.DecisionSHA256}
}

func sealShadowComparison(document map[string]any) ([]byte, string, []byte, error) {
	comparisonJSON, err := canonicaljson.Marshal(document)
	if err != nil {
		return nil, "", nil, fmt.Errorf("canonicalize comparison: %w", err)
	}
	comparisonDigest, err := canonicaljson.Digest(canonicaljson.DomainShadowComparison, document)
	if err != nil {
		return nil, "", nil, fmt.Errorf("digest comparison: %w", err)
	}
	comparisonSHA, err := canonicaljson.DecodeDigest(comparisonDigest)
	if err != nil {
		return nil, "", nil, fmt.Errorf("decode comparison digest: %w", err)
	}
	return comparisonJSON, comparisonDigest, comparisonSHA, nil
}

func shadowInputDigests(input ShadowInput) (shadowDigests, error) {
	snapshotSHA, err := canonicaljson.DecodeDigest(input.SnapshotDigest)
	if err != nil {
		return shadowDigests{}, fmt.Errorf("decode snapshot digest: %w", err)
	}
	specSHA, err := canonicaljson.DecodeDigest(input.SpecDigest)
	if err != nil {
		return shadowDigests{}, fmt.Errorf("decode spec digest: %w", err)
	}
	policySHA, err := canonicaljson.DecodeDigest(input.PolicyDigest)
	if err != nil {
		return shadowDigests{}, fmt.Errorf("decode policy digest: %w", err)
	}
	return shadowDigests{snapshot: snapshotSHA, spec: specSHA, policy: policySHA}, nil
}

func assembleShadowComparison(input ShadowInput, baseline, tamoz validatedShadowOutput, tenantID, comparisonKey, comparisonID string, comparisonJSON []byte, comparisonDigest string, comparisonSHA []byte, digests shadowDigests, createdAt time.Time) builtShadowComparison {
	return builtShadowComparison{
		record: shadowComparisonRecord(input, baseline, tamoz, tenantID, comparisonKey, comparisonID, comparisonJSON, comparisonSHA, digests, createdAt),
		result: ShadowComparisonResult{EpisodeKey: input.EpisodeKey, ComparisonSHA256: comparisonDigest,
			BaselineDecisionSHA256: baseline.output.DecisionSHA256, TamozDecisionSHA256: tamoz.output.DecisionSHA256,
			DecisionsEqual: bytes.Equal(baseline.canonical, tamoz.canonical)},
	}
}

func shadowComparisonRecord(input ShadowInput, baseline, tamoz validatedShadowOutput, tenantID, comparisonKey, comparisonID string, comparisonJSON, comparisonSHA []byte, digests shadowDigests, createdAt time.Time) qualification.ShadowComparison {
	return qualification.ShadowComparison{
		ComparisonID: comparisonID, ComparisonKey: comparisonKey, TenantID: tenantID,
		EpisodeID: input.EpisodeID, SituationID: input.SituationID, SituationVersion: input.SituationVersion,
		TriggerID: input.TriggerID, SnapshotSHA256: digests.snapshot, SpecSHA256: digests.spec, PolicySHA256: digests.policy,
		BaselineExecutorVersion: baseline.output.ExecutorVersion, TamozExecutorVersion: tamoz.output.ExecutorVersion,
		BaselineManifestSHA256: baseline.manifestSHA, TamozManifestSHA256: tamoz.manifestSHA,
		BaselineDecisionJSON: baseline.canonical, BaselineDecisionSHA256: baseline.decisionSHA,
		TamozDecisionJSON: tamoz.canonical, TamozDecisionSHA256: tamoz.decisionSHA,
		ComparisonJSON: comparisonJSON, ComparisonSHA256: comparisonSHA, CreatedAt: createdAt.UTC().Format(time.RFC3339Nano),
	}
}
