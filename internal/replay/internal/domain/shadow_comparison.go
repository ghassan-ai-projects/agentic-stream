package domain

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/canonicaljson"
)

// ShadowComparisonResult identifies the durable report produced for one
// paired shadow trial.
type ShadowComparisonResult struct {
	EpisodeKey             string
	ComparisonSHA256       string
	BaselineDecisionSHA256 string
	TamozDecisionSHA256    string
	DecisionsEqual         bool
}

// Comparison is the sealed, report-only record of one shadow trial, ready for
// the replay store; it never enters intents, commands or the outbox.
type Comparison struct {
	ComparisonID            string
	ComparisonKey           string
	TenantID                string
	EpisodeID               string
	SituationID             string
	SituationVersion        int
	TriggerID               string
	SnapshotSHA256          []byte
	SpecSHA256              []byte
	PolicySHA256            []byte
	BaselineExecutorVersion string
	TamozExecutorVersion    string
	BaselineManifestSHA256  []byte
	TamozManifestSHA256     []byte
	BaselineDecisionJSON    []byte
	BaselineDecisionSHA256  []byte
	TamozDecisionJSON       []byte
	TamozDecisionSHA256     []byte
	ComparisonJSON          []byte
	ComparisonSHA256        []byte
	CreatedAt               string
}

type builtComparison struct {
	record Comparison
	result ShadowComparisonResult
}

type comparisonDigests struct{ snapshot, spec, policy []byte }

// BuildComparison seals one paired shadow trial as a durable comparison and
// its result summary. Wall time enters only the record's creation timestamp.
func BuildComparison(input ShadowInput, baseline, tamoz ValidatedOutput, tenantID string, createdAt time.Time) (Comparison, ShadowComparisonResult, error) {
	comparisonKey := tenantID + ":" + input.EpisodeKey
	document := comparisonDocument(input, baseline, tamoz, tenantID, comparisonKey)
	comparisonJSON, comparisonDigest, comparisonSHA, err := sealComparison(document)
	if err != nil {
		return Comparison{}, ShadowComparisonResult{}, err
	}
	digests, err := inputDigests(input)
	if err != nil {
		return Comparison{}, ShadowComparisonResult{}, err
	}
	comparisonID := "cmp_" + hex.EncodeToString(comparisonSHA)
	built := assembleComparison(input, baseline, tamoz, tenantID, comparisonKey, comparisonID, comparisonJSON, comparisonDigest, comparisonSHA, digests, createdAt)
	return built.record, built.result, nil
}

func comparisonDocument(input ShadowInput, baseline, tamoz ValidatedOutput, tenantID, comparisonKey string) map[string]any {
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
		"baseline":          outputProvenance(baseline),
		"tamoz":             outputProvenance(tamoz),
		"differences":       outputDifferences(baseline, tamoz),
	}
}

func outputDifferences(baseline, tamoz ValidatedOutput) []string {
	differences := make([]string, 0, 2)
	if !bytes.Equal(baseline.Canonical, tamoz.Canonical) {
		differences = append(differences, "decision")
	}
	if baseline.Output.ManifestSHA256 != tamoz.Output.ManifestSHA256 {
		differences = append(differences, "manifest")
	}
	return differences
}

func outputProvenance(value ValidatedOutput) map[string]any {
	return map[string]any{"executor_version": value.Output.ExecutorVersion, "manifest_sha256": value.Output.ManifestSHA256, "decision_sha256": value.Output.DecisionSHA256}
}

func sealComparison(document map[string]any) ([]byte, string, []byte, error) {
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

func inputDigests(input ShadowInput) (comparisonDigests, error) {
	snapshotSHA, err := canonicaljson.DecodeDigest(input.SnapshotDigest)
	if err != nil {
		return comparisonDigests{}, fmt.Errorf("decode snapshot digest: %w", err)
	}
	specSHA, err := canonicaljson.DecodeDigest(input.SpecDigest)
	if err != nil {
		return comparisonDigests{}, fmt.Errorf("decode spec digest: %w", err)
	}
	policySHA, err := canonicaljson.DecodeDigest(input.PolicyDigest)
	if err != nil {
		return comparisonDigests{}, fmt.Errorf("decode policy digest: %w", err)
	}
	return comparisonDigests{snapshot: snapshotSHA, spec: specSHA, policy: policySHA}, nil
}

func assembleComparison(input ShadowInput, baseline, tamoz ValidatedOutput, tenantID, comparisonKey, comparisonID string, comparisonJSON []byte, comparisonDigest string, comparisonSHA []byte, digests comparisonDigests, createdAt time.Time) builtComparison {
	return builtComparison{
		record: comparisonRecord(input, baseline, tamoz, tenantID, comparisonKey, comparisonID, comparisonJSON, comparisonSHA, digests, createdAt),
		result: ShadowComparisonResult{EpisodeKey: input.EpisodeKey, ComparisonSHA256: comparisonDigest,
			BaselineDecisionSHA256: baseline.Output.DecisionSHA256, TamozDecisionSHA256: tamoz.Output.DecisionSHA256,
			DecisionsEqual: bytes.Equal(baseline.Canonical, tamoz.Canonical)},
	}
}

func comparisonRecord(input ShadowInput, baseline, tamoz ValidatedOutput, tenantID, comparisonKey, comparisonID string, comparisonJSON, comparisonSHA []byte, digests comparisonDigests, createdAt time.Time) Comparison {
	return Comparison{
		ComparisonID: comparisonID, ComparisonKey: comparisonKey, TenantID: tenantID,
		EpisodeID: input.EpisodeID, SituationID: input.SituationID, SituationVersion: input.SituationVersion,
		TriggerID: input.TriggerID, SnapshotSHA256: digests.snapshot, SpecSHA256: digests.spec, PolicySHA256: digests.policy,
		BaselineExecutorVersion: baseline.Output.ExecutorVersion, TamozExecutorVersion: tamoz.Output.ExecutorVersion,
		BaselineManifestSHA256: baseline.ManifestSHA, TamozManifestSHA256: tamoz.ManifestSHA,
		BaselineDecisionJSON: baseline.Canonical, BaselineDecisionSHA256: baseline.DecisionSHA,
		TamozDecisionJSON: tamoz.Canonical, TamozDecisionSHA256: tamoz.DecisionSHA,
		ComparisonJSON: comparisonJSON, ComparisonSHA256: comparisonSHA, CreatedAt: createdAt.UTC().Format(time.RFC3339Nano),
	}
}
