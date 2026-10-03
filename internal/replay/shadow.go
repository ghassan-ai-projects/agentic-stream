package replay

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/qualification"

	"github.com/ghassan-ai-projects/agentic-stream/internal/decisions"
	"github.com/ghassan-ai-projects/agentic-stream/internal/episodes"
	"github.com/ghassan-ai-projects/agentic-stream/internal/policy"
	"github.com/ghassan-ai-projects/agentic-stream/internal/spec"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
)

type shadowValidation struct {
	catalog                   *decisions.IntentCatalog
	allowedTypes              map[string]struct{}
	riskCeiling, policyDigest string
}

func applyPairedShadow(ctx context.Context, db *storage.DB, tenantID string, caps Capabilities, compiled *spec.CompiledSpec, items []replayItem, evaluationTime time.Time, result *Result) error {
	validation, err := compileShadowValidation(compiled)
	if err != nil {
		return err
	}
	store := qualification.ShadowComparisonStore{}
	for _, item := range items {
		if err := compareShadowEpisode(ctx, db, tenantID, caps, compiled, item, validation, evaluationTime, store, result); err != nil {
			return err
		}
	}
	return nil
}

func compileShadowValidation(compiled *spec.CompiledSpec) (shadowValidation, error) {
	if compiled == nil {
		return shadowValidation{}, fmt.Errorf("shadow comparison requires compiled spec")
	}
	catalog, err := compileShadowCatalog(compiled.Actions.Intents)
	if err != nil {
		return shadowValidation{}, err
	}
	allowedTypes := shadowAllowedTypes(compiled.Actions.Intents)
	policyDigest, err := policy.DigestForVersion(compiled.Digest)
	if err != nil {
		return shadowValidation{}, fmt.Errorf("digest shadow policy: %w", err)
	}
	return shadowValidation{catalog: catalog, allowedTypes: allowedTypes, riskCeiling: compiled.Cognition.Executor.RiskCeiling, policyDigest: policyDigest}, nil
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

func compareShadowEpisode(ctx context.Context, db *storage.DB, tenantID string, caps Capabilities, compiled *spec.CompiledSpec, item replayItem, validation shadowValidation, evaluationTime time.Time, store qualification.ShadowComparisonStore, result *Result) error {
	input, err := loadShadowInput(ctx, db, item, tenantID, compiled.Digest, validation.policyDigest, evaluationTime)
	if err != nil {
		return err
	}
	baseline, tamoz, err := evaluateShadowPair(ctx, input, caps, validation, evaluationTime, result)
	if err != nil {
		return err
	}
	comparison, err := buildShadowComparison(input, baseline, tamoz, tenantID, evaluationTime)
	if err != nil {
		return fmt.Errorf("build shadow comparison %s: %w", input.EpisodeKey, err)
	}
	return recordShadowComparison(ctx, db, store, input, comparison, result)

}

func evaluateShadowPair(ctx context.Context, input ShadowInput, caps Capabilities, validation shadowValidation, evaluationTime time.Time, result *Result) (validatedShadowOutput, validatedShadowOutput, error) {
	baselineInput := cloneShadowInput(input)
	tamozInput := cloneShadowInput(input)
	baselineOutput, err := caps.BaselineExecutor.ExecuteBaseline(ctx, baselineInput)
	if err != nil {
		return validatedShadowOutput{}, validatedShadowOutput{}, fmt.Errorf("baseline shadow episode %s: %w", input.EpisodeKey, err)
	}
	tamozOutput, err := caps.ShadowExecutor.ExecuteShadow(ctx, tamozInput)
	if err != nil {
		return validatedShadowOutput{}, validatedShadowOutput{}, fmt.Errorf("tamoz shadow episode %s: %w", input.EpisodeKey, err)
	}
	result.WorkerInvoked = true
	result.CapabilityCalls += 2
	return validateShadowPair(input, baselineOutput, tamozOutput, validation, evaluationTime)
}

func recordShadowComparison(ctx context.Context, db *storage.DB, store qualification.ShadowComparisonStore, input ShadowInput, comparison builtShadowComparison, result *Result) error {
	if err := db.WithTx(ctx, func(tx *sql.Tx) error {
		return store.Record(ctx, tx, comparison.record)
	}); err != nil {
		return fmt.Errorf("persist shadow comparison %s: %w", input.EpisodeKey, err)
	}
	result.ShadowComparisons = append(result.ShadowComparisons, comparison.result)
	if !comparison.result.DecisionsEqual {
		result.Findings = append(result.Findings, Finding{Code: "shadow_decision_diff", Message: input.EpisodeKey})
	}
	return nil
}
