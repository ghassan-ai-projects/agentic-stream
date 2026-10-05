package replay

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/decisions"
	"github.com/ghassan-ai-projects/agentic-stream/internal/episodes"
	"github.com/ghassan-ai-projects/agentic-stream/internal/policy"
	"github.com/ghassan-ai-projects/agentic-stream/internal/qualification"
	domain "github.com/ghassan-ai-projects/agentic-stream/internal/replay/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/spec"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
)

func applyPairedShadow(ctx context.Context, db *storage.DB, tenantID string, caps Capabilities, compiled *spec.CompiledSpec, episodes []domain.ReplayEpisode, evaluationTime time.Time, result *Result) error {
	rules, err := compileShadowRules(compiled)
	if err != nil {
		return err
	}
	store := qualification.ShadowComparisonStore{}
	for _, episode := range episodes {
		if err := compareShadowEpisode(ctx, db, tenantID, caps, compiled, episode, rules, evaluationTime, store, result); err != nil {
			return err
		}
	}
	return nil
}

func compileShadowRules(compiled *spec.CompiledSpec) (domain.ShadowRules, error) {
	if compiled == nil {
		return domain.ShadowRules{}, fmt.Errorf("shadow comparison requires compiled spec")
	}
	catalog, err := compileShadowCatalog(compiled.Actions.Intents)
	if err != nil {
		return domain.ShadowRules{}, err
	}
	allowedTypes := shadowAllowedTypes(compiled.Actions.Intents)
	policyDigest, err := policy.DigestForVersion(compiled.Digest)
	if err != nil {
		return domain.ShadowRules{}, fmt.Errorf("digest shadow policy: %w", err)
	}
	return domain.ShadowRules{Catalog: catalog, AllowedTypes: allowedTypes, RiskCeiling: compiled.Cognition.Executor.RiskCeiling, PolicyDigest: policyDigest}, nil
}

func compileShadowCatalog(intents []spec.Intent) (*decisions.IntentCatalog, error) {
	catalogDocument, _, err := episodes.CompileIntentCatalog(intents)
	if err != nil {
		return nil, fmt.Errorf("compile shadow intent catalog: %w", err)
	}
	catalog, err := decisions.CompileIntentCatalog(catalogDocument)
	if err != nil {
		return nil, fmt.Errorf("compile shadow decision catalog: %w", err)
	}
	return catalog, nil
}

func shadowAllowedTypes(intents []spec.Intent) map[string]struct{} {
	allowedTypes := make(map[string]struct{}, len(intents))
	for _, intent := range intents {
		allowedTypes[intent.Type] = struct{}{}
	}
	return allowedTypes
}

func compareShadowEpisode(ctx context.Context, db *storage.DB, tenantID string, caps Capabilities, compiled *spec.CompiledSpec, episode domain.ReplayEpisode, rules domain.ShadowRules, evaluationTime time.Time, store qualification.ShadowComparisonStore, result *Result) error {
	input, err := loadShadowInput(ctx, db, episode, tenantID, compiled.Digest, rules.PolicyDigest, evaluationTime)
	if err != nil {
		return err
	}
	baseline, tamoz, err := evaluateShadowPair(ctx, input, caps, rules, evaluationTime, result)
	if err != nil {
		return err
	}
	comparison, comparisonResult, err := domain.BuildComparison(input, baseline, tamoz, tenantID, evaluationTime)
	if err != nil {
		return fmt.Errorf("build shadow comparison %s: %w", input.EpisodeKey, err)
	}
	return recordShadowComparison(ctx, db, store, input, comparison, comparisonResult, result)
}

func evaluateShadowPair(ctx context.Context, input ShadowInput, caps Capabilities, rules domain.ShadowRules, evaluationTime time.Time, result *Result) (domain.ValidatedOutput, domain.ValidatedOutput, error) {
	baselineOutput, err := caps.BaselineExecutor.ExecuteBaseline(ctx, input.Clone())
	if err != nil {
		return domain.ValidatedOutput{}, domain.ValidatedOutput{}, fmt.Errorf("baseline shadow episode %s: %w", input.EpisodeKey, err)
	}
	tamozOutput, err := caps.ShadowExecutor.ExecuteShadow(ctx, input.Clone())
	if err != nil {
		return domain.ValidatedOutput{}, domain.ValidatedOutput{}, fmt.Errorf("tamoz shadow episode %s: %w", input.EpisodeKey, err)
	}
	result.WorkerInvoked = true
	result.CapabilityCalls += 2
	return rules.ValidatePair(input, baselineOutput, tamozOutput, evaluationTime)
}

func recordShadowComparison(ctx context.Context, db *storage.DB, store qualification.ShadowComparisonStore, input ShadowInput, comparison domain.Comparison, comparisonResult ShadowComparisonResult, result *Result) error {
	if err := db.WithTx(ctx, func(tx *sql.Tx) error {
		return store.Record(ctx, tx, qualificationRecord(comparison))
	}); err != nil {
		return fmt.Errorf("persist shadow comparison %s: %w", input.EpisodeKey, err)
	}
	result.ShadowComparisons = append(result.ShadowComparisons, comparisonResult)
	if !comparisonResult.DecisionsEqual {
		result.Findings = append(result.Findings, Finding{Code: "shadow_decision_diff", Message: input.EpisodeKey})
	}
	return nil
}

func qualificationRecord(comparison domain.Comparison) qualification.ShadowComparison {
	return qualification.ShadowComparison{
		ComparisonID: comparison.ComparisonID, ComparisonKey: comparison.ComparisonKey, TenantID: comparison.TenantID,
		EpisodeID: comparison.EpisodeID, SituationID: comparison.SituationID, SituationVersion: comparison.SituationVersion,
		TriggerID: comparison.TriggerID, SnapshotSHA256: comparison.SnapshotSHA256, SpecSHA256: comparison.SpecSHA256, PolicySHA256: comparison.PolicySHA256,
		BaselineExecutorVersion: comparison.BaselineExecutorVersion, TamozExecutorVersion: comparison.TamozExecutorVersion,
		BaselineManifestSHA256: comparison.BaselineManifestSHA256, TamozManifestSHA256: comparison.TamozManifestSHA256,
		BaselineDecisionJSON: comparison.BaselineDecisionJSON, BaselineDecisionSHA256: comparison.BaselineDecisionSHA256,
		TamozDecisionJSON: comparison.TamozDecisionJSON, TamozDecisionSHA256: comparison.TamozDecisionSHA256,
		ComparisonJSON: comparison.ComparisonJSON, ComparisonSHA256: comparison.ComparisonSHA256, CreatedAt: comparison.CreatedAt,
	}
}
