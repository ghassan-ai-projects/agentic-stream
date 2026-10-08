package app

import (
	"context"
	"fmt"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/decisions"
	"github.com/ghassan-ai-projects/agentic-stream/internal/episodes"
	"github.com/ghassan-ai-projects/agentic-stream/internal/policy"
	domain "github.com/ghassan-ai-projects/agentic-stream/internal/replay/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/spec"
)

func applyPairedShadow(ctx context.Context, session *replaySession, caps domain.Capabilities, episodes []domain.ReplayEpisode, evaluationTime time.Time, result *domain.Result) error {
	rules, err := compileShadowRules(session.compiled)
	if err != nil {
		return err
	}
	for _, episode := range episodes {
		if err := compareShadowEpisode(ctx, session, caps, episode, rules, evaluationTime, result); err != nil {
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

// compareShadowEpisode runs one paired trial. The baseline is this
// repository's own code, so its failure fails the run; a candidate that fails
// or answers outside the contract is a finding about the candidate, and the
// trial continues with the next episode.
func compareShadowEpisode(ctx context.Context, session *replaySession, caps domain.Capabilities, episode domain.ReplayEpisode, rules domain.ShadowRules, evaluationTime time.Time, result *domain.Result) error {
	input, err := loadShadowInput(ctx, session, episode, rules.PolicyDigest, evaluationTime)
	if err != nil {
		return err
	}
	baseline, err := evaluateBaseline(ctx, caps, input, rules, evaluationTime)
	if err != nil {
		return err
	}
	tamoz, err := evaluateCandidate(ctx, caps, input, rules, evaluationTime, result)
	if err != nil || tamoz == nil {
		return err
	}
	return recordShadowComparison(ctx, session, input, baseline, *tamoz, evaluationTime, result)
}

func loadShadowInput(ctx context.Context, session *replaySession, episode domain.ReplayEpisode, policyDigest string, evaluationTime time.Time) (domain.ShadowInput, error) {
	snapshot, persistedDigest, err := session.store.ShadowSnapshot(ctx, episode)
	if err != nil {
		return domain.ShadowInput{}, err
	}
	canonical, err := domain.VerifiedSnapshot(snapshot, persistedDigest)
	if err != nil {
		return domain.ShadowInput{}, err
	}
	input := newShadowInput(episode, session.tenantID, session.compiled.Digest, policyDigest, canonical, evaluationTime)
	input.Request, err = session.store.ShadowRequest(ctx, episode)
	return input, err
}

func newShadowInput(episode domain.ReplayEpisode, tenantID, specDigest, policyDigest string, canonical []byte, evaluationTime time.Time) domain.ShadowInput {
	return domain.ShadowInput{
		TenantID: tenantID, EpisodeKey: episode.EpisodeKey,
		EpisodeID: episode.EpisodeID, SituationID: episode.SituationID, SituationVersion: episode.SituationVersion,
		TriggerID: episode.TriggerID, AttemptID: "shadow-attempt/" + episode.EpisodeID, Fence: 1,
		SnapshotDigest: episode.SnapshotDigest, SpecDigest: specDigest, PolicyDigest: policyDigest,
		SnapshotJSON: append([]byte(nil), canonical...), EvaluationTime: evaluationTime,
	}
}

func evaluateBaseline(ctx context.Context, caps domain.Capabilities, input domain.ShadowInput, rules domain.ShadowRules, evaluationTime time.Time) (domain.ValidatedOutput, error) {
	output, err := caps.BaselineExecutor.ExecuteBaseline(ctx, input.Clone())
	if err != nil {
		return domain.ValidatedOutput{}, fmt.Errorf("baseline shadow episode %s: %w", input.EpisodeKey, err)
	}
	validated, err := rules.ValidateOutput(input, output, evaluationTime)
	if err != nil {
		return domain.ValidatedOutput{}, fmt.Errorf("validate baseline shadow episode %s: %w", input.EpisodeKey, err)
	}
	return validated, nil
}

// evaluateCandidate returns nil and records a finding when the candidate
// fails or its output is invalid. Cancellation still stops the run.
func evaluateCandidate(ctx context.Context, caps domain.Capabilities, input domain.ShadowInput, rules domain.ShadowRules, evaluationTime time.Time, result *domain.Result) (*domain.ValidatedOutput, error) {
	output, err := caps.ShadowExecutor.ExecuteShadow(ctx, input.Clone())
	result.WorkerInvoked = true
	result.CapabilityCalls += 2
	if ctx.Err() != nil {
		return nil, fmt.Errorf("tamoz shadow episode %s: %w", input.EpisodeKey, ctx.Err())
	}
	if err != nil {
		return nil, candidateFinding(result, "shadow_candidate_failed", input, err)
	}
	validated, err := rules.ValidateOutput(input, output, evaluationTime)
	if err != nil {
		return nil, candidateFinding(result, "shadow_candidate_invalid", input, err)
	}
	return &validated, nil
}

// candidateFinding records a candidate's failure as a result of the trial;
// it always returns nil so the trial moves on.
func candidateFinding(result *domain.Result, code string, input domain.ShadowInput, cause error) error {
	result.Findings = append(result.Findings, domain.Finding{Code: code, Message: input.EpisodeKey + ": " + cause.Error()})
	return nil
}

func recordShadowComparison(ctx context.Context, session *replaySession, input domain.ShadowInput, baseline, tamoz domain.ValidatedOutput, evaluationTime time.Time, result *domain.Result) error {
	comparison, comparisonResult, err := domain.BuildComparison(input, baseline, tamoz, session.tenantID, evaluationTime)
	if err != nil {
		return fmt.Errorf("build shadow comparison %s: %w", input.EpisodeKey, err)
	}
	if err := session.store.RecordShadowComparison(ctx, comparison); err != nil {
		return fmt.Errorf("persist shadow comparison %s: %w", input.EpisodeKey, err)
	}
	result.ShadowComparisons = append(result.ShadowComparisons, comparisonResult)
	if !comparisonResult.DecisionsEqual {
		result.Findings = append(result.Findings, domain.Finding{Code: "shadow_decision_diff", Message: input.EpisodeKey})
	}
	return nil
}
