package replay

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/canonicaljson"
	"github.com/ghassan-ai-projects/agentic-stream/internal/contractsv1"
	"github.com/ghassan-ai-projects/agentic-stream/internal/decisions"
	"github.com/ghassan-ai-projects/agentic-stream/internal/episodes"
	"github.com/ghassan-ai-projects/agentic-stream/internal/policy"
	"github.com/ghassan-ai-projects/agentic-stream/internal/spec"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
)

func applyPairedShadow(ctx context.Context, db *storage.DB, tenantID string, caps Capabilities, compiled *spec.CompiledSpec, items []replayItem, evaluationTime time.Time, result *Result) error {
	if compiled == nil {
		return fmt.Errorf("shadow comparison requires compiled spec")
	}
	catalogDocument, _, err := episodes.CompileIntentCatalog(compiled.Actions.Intents)
	if err != nil {
		return fmt.Errorf("compile shadow intent catalog: %w", err)
	}
	catalog, err := decisions.CompileIntentCatalog(catalogDocument)
	if err != nil {
		return fmt.Errorf("compile shadow decision catalog: %w", err)
	}
	allowedTypes := make(map[string]struct{}, len(compiled.Actions.Intents))
	for _, intent := range compiled.Actions.Intents {
		allowedTypes[intent.Type] = struct{}{}
	}
	policyDigest, err := policy.DigestForVersion(compiled.Digest)
	if err != nil {
		return fmt.Errorf("digest shadow policy: %w", err)
	}
	store := storage.ShadowComparisonStore{}
	for _, item := range items {
		input, err := loadShadowInput(ctx, db, item, tenantID, compiled.Digest, policyDigest, evaluationTime)
		if err != nil {
			return err
		}
		baselineInput := cloneShadowInput(input)
		tamozInput := cloneShadowInput(input)
		baselineOutput, err := caps.BaselineExecutor.ExecuteBaseline(ctx, baselineInput)
		if err != nil {
			return fmt.Errorf("baseline shadow episode %s: %w", input.EpisodeKey, err)
		}
		tamozOutput, err := caps.ShadowExecutor.ExecuteShadow(ctx, tamozInput)
		if err != nil {
			return fmt.Errorf("tamoz shadow episode %s: %w", input.EpisodeKey, err)
		}
		result.WorkerInvoked = true
		result.CapabilityCalls += 2
		baseline, err := validateShadowOutput(input, baselineOutput, catalog, allowedTypes, compiled.Cognition.Executor.RiskCeiling, evaluationTime)
		if err != nil {
			return fmt.Errorf("validate baseline shadow episode %s: %w", input.EpisodeKey, err)
		}
		tamoz, err := validateShadowOutput(input, tamozOutput, catalog, allowedTypes, compiled.Cognition.Executor.RiskCeiling, evaluationTime)
		if err != nil {
			return fmt.Errorf("validate Tamoz shadow episode %s: %w", input.EpisodeKey, err)
		}
		comparison, err := buildShadowComparison(input, baseline, tamoz, tenantID, evaluationTime)
		if err != nil {
			return fmt.Errorf("build shadow comparison %s: %w", input.EpisodeKey, err)
		}
		if err := db.WithTx(ctx, func(tx *sql.Tx) error {
			return store.Record(ctx, tx, comparison.record)
		}); err != nil {
			return fmt.Errorf("persist shadow comparison %s: %w", input.EpisodeKey, err)
		}
		result.ShadowComparisons = append(result.ShadowComparisons, comparison.result)
		if !comparison.result.DecisionsEqual {
			result.Findings = append(result.Findings, Finding{Code: "shadow_decision_diff", Message: input.EpisodeKey})
		}
	}
	return nil
}

type validatedShadowOutput struct {
	output      ShadowOutput
	canonical   []byte
	decision    *decisions.Result
	decisionSHA []byte
	manifestSHA []byte
}

func validateShadowOutput(input ShadowInput, output ShadowOutput, catalog *decisions.IntentCatalog, allowedTypes map[string]struct{}, riskCeiling string, now time.Time) (validatedShadowOutput, error) {
	if output.ExecutorVersion == "" {
		return validatedShadowOutput{}, fmt.Errorf("executor version is required")
	}
	manifestSHA, err := canonicaljson.DecodeDigest(output.ManifestSHA256)
	if err != nil {
		return validatedShadowOutput{}, fmt.Errorf("manifest digest: %w", err)
	}
	canonical, err := canonicaljson.Marshal(json.RawMessage(output.DecisionJSON))
	if err != nil {
		return validatedShadowOutput{}, fmt.Errorf("decision JSON: %w", err)
	}
	if !bytes.Equal(canonical, output.DecisionJSON) {
		return validatedShadowOutput{}, fmt.Errorf("decision JSON is not canonical")
	}
	var document map[string]any
	if err := json.Unmarshal(canonical, &document); err != nil {
		return validatedShadowOutput{}, fmt.Errorf("decode decision JSON: %w", err)
	}
	decisionSHA, err := canonicaljson.DecodeDigest(output.DecisionSHA256)
	if err != nil {
		return validatedShadowOutput{}, fmt.Errorf("decision digest: %w", err)
	}
	if !canonicaljson.Verify(canonicaljson.DomainDecision, document, output.DecisionSHA256) {
		return validatedShadowOutput{}, fmt.Errorf("decision digest does not match decision JSON")
	}
	entityID, err := shadowEntityID(input.SnapshotJSON)
	if err != nil {
		return validatedShadowOutput{}, err
	}
	if riskCeiling == "" {
		riskCeiling = "R1"
	}
	validated, err := decisions.Validate(canonical, output.DecisionSHA256, decisions.Input{
		EpisodeID: input.EpisodeID, AttemptID: input.AttemptID, Fence: input.Fence,
		TenantID: input.TenantID, SituationID: input.SituationID,
		SituationVersion: input.SituationVersion, EntityID: entityID, SnapshotDigest: input.SnapshotDigest,
		AllowedIntentTypes: allowedTypes, RiskCeiling: riskCeiling, IntentCatalog: catalog,
		Kind: "standard", Now: now,
	})
	if err != nil {
		return validatedShadowOutput{}, fmt.Errorf("validate shadow decision: %w", err)
	}
	return validatedShadowOutput{output: output, canonical: canonical, decision: validated, decisionSHA: decisionSHA, manifestSHA: manifestSHA}, nil
}

func cloneShadowInput(input ShadowInput) ShadowInput {
	input.SnapshotJSON = append([]byte(nil), input.SnapshotJSON...)
	return input
}

type builtShadowComparison struct {
	record storage.ShadowComparison
	result ShadowComparisonResult
}

func buildShadowComparison(input ShadowInput, baseline, tamoz validatedShadowOutput, tenantID string, createdAt time.Time) (builtShadowComparison, error) {
	differences := make([]string, 0, 2)
	if !bytes.Equal(baseline.canonical, tamoz.canonical) {
		differences = append(differences, "decision")
	}
	if baseline.output.ManifestSHA256 != tamoz.output.ManifestSHA256 {
		differences = append(differences, "manifest")
	}
	comparisonKey := tenantID + ":" + input.EpisodeKey
	document := map[string]any{
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
		record: storage.ShadowComparison{
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

func loadShadowInput(ctx context.Context, db *storage.DB, item replayItem, tenantID, specDigest, policyDigest string, evaluationTime time.Time) (ShadowInput, error) {
	var snapshot, persistedDigest []byte
	if err := db.QueryRowContext(ctx, `
		SELECT snapshot_json, snapshot_sha256 FROM situation_versions
		WHERE situation_id = ? AND version = ?`, item.SituationID, item.SituationVersion).Scan(&snapshot, &persistedDigest); err != nil {
		return ShadowInput{}, fmt.Errorf("load shadow snapshot: %w", err)
	}
	var document map[string]any
	canonical, err := canonicaljson.Marshal(json.RawMessage(snapshot))
	if err != nil {
		return ShadowInput{}, fmt.Errorf("canonicalize shadow snapshot: %w", err)
	}
	if err := json.Unmarshal(canonical, &document); err != nil {
		return ShadowInput{}, fmt.Errorf("decode shadow snapshot: %w", err)
	}
	if err := contractsv1.Validate(contractsv1.SchemaSnapshot, document); err != nil {
		return ShadowInput{}, fmt.Errorf("validate shadow snapshot: %w", err)
	}
	digest, err := canonicaljson.Digest(canonicaljson.DomainSnapshot, document)
	if err != nil {
		return ShadowInput{}, fmt.Errorf("digest shadow snapshot: %w", err)
	}
	decoded, err := canonicaljson.DecodeDigest(digest)
	if err != nil || !bytes.Equal(decoded, persistedDigest) {
		return ShadowInput{}, fmt.Errorf("shadow snapshot digest mismatch")
	}
	return ShadowInput{
		TenantID: tenantID, EpisodeKey: replayEpisodeKey(item.SituationID, item.SituationVersion, item.TriggerID),
		EpisodeID: item.EpisodeID, SituationID: item.SituationID, SituationVersion: item.SituationVersion,
		TriggerID: item.TriggerID, AttemptID: "shadow-attempt/" + item.EpisodeID, Fence: 1,
		SnapshotDigest: item.SnapshotDigest, SpecDigest: specDigest, PolicyDigest: policyDigest,
		SnapshotJSON:   append([]byte(nil), canonical...),
		EvaluationTime: evaluationTime,
	}, nil
}

func shadowEntityID(snapshot []byte) (string, error) {
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
