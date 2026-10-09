package app

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/contractsv1"
	domain "github.com/ghassan-ai-projects/agentic-stream/internal/replay/internal/domain"
)

func runShadow(t *testing.T, request domain.Request, baseline *baselineExecutor, candidate domain.ShadowExecutor) (domain.Result, error) {
	t.Helper()
	return RunMode(seededContext(t), domain.ModeShadow, request, domain.Capabilities{BaselineExecutor: baseline, ShadowExecutor: candidate})
}

func TestShadowPhasePairsBaselineWithCandidateOnEveryEpisode(t *testing.T) {
	t.Parallel()
	baseline, candidate := &baselineExecutor{}, &candidateExecutor{}
	result, err := runShadow(t, newRequest(t, alwaysTriggerSpec(t)), baseline, candidate)
	if err != nil {
		t.Fatal(err)
	}
	if result.Mode != domain.ModeShadow || !result.WorkerInvoked || result.EffectsAllowed {
		t.Fatalf("shadow result = %+v", result)
	}
	if len(result.ShadowComparisons) == 0 || result.CapabilityCalls != 2*len(result.ShadowComparisons) || baseline.calls != candidate.calls || baseline.calls+candidate.calls != result.CapabilityCalls {
		t.Fatalf("shadow accounting: result=%+v baseline=%d candidate=%d", result, baseline.calls, candidate.calls)
	}
	if len(result.Findings) != 0 {
		t.Fatalf("identical decisions produced findings: %+v", result.Findings)
	}
}

func TestShadowPhasePersistsOnlyComparisonsAndRepeatsByteForByte(t *testing.T) {
	t.Parallel()
	workingSpec := alwaysTriggerSpec(t)
	baseline, candidate := &baselineExecutor{mutateInput: true}, &candidateExecutor{}
	request := newRequest(t, workingSpec)
	first, err := runShadow(t, request, baseline, candidate)
	if err != nil {
		t.Fatal(err)
	}
	for index := range baseline.snapshots {
		if string(baseline.snapshots[index]) != string(candidate.snapshots[index]) {
			t.Fatalf("the candidate saw a snapshot the baseline mutated, at index %d", index)
		}
	}
	db := openReplayDatabase(t, request.DBPath)
	if comparisons := countRows(t, db, "shadow_comparisons"); comparisons != len(first.ShadowComparisons) || comparisons == 0 {
		t.Fatalf("persisted comparisons = %d, result reports %d", comparisons, len(first.ShadowComparisons))
	}
	for _, table := range []string{"intents", "commands", "outbox"} {
		if rows := countRows(t, db, table); rows != 0 {
			t.Fatalf("shadow replay wrote %d rows into %s", rows, table)
		}
	}
	var artifact, digest []byte
	if err := db.QueryRowContext(t.Context(), "SELECT comparison_json, comparison_sha256 FROM shadow_comparisons LIMIT 1").Scan(&artifact, &digest); err != nil {
		t.Fatal(err)
	}
	if len(artifact) == 0 || len(digest) != 32 {
		t.Fatalf("comparison artifact is incomplete: json=%d digest=%d", len(artifact), len(digest))
	}
	repeat, err := runShadow(t, newRequest(t, workingSpec), &baselineExecutor{}, &candidateExecutor{})
	if err != nil {
		t.Fatal(err)
	}
	if got, want := repeat.ShadowComparisons[0].ComparisonSHA256, first.ShadowComparisons[0].ComparisonSHA256; got != want {
		t.Fatalf("comparison digest of the repeat = %s, want %s", got, want)
	}
}

func TestShadowPhaseReportsADecisionDifferenceAsAFinding(t *testing.T) {
	t.Parallel()
	result, err := runShadow(t, newRequest(t, alwaysTriggerSpec(t)), &baselineExecutor{summary: "baseline"}, &candidateExecutor{summary: "tamoz"})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.ShadowComparisons) != 1 || result.ShadowComparisons[0].DecisionsEqual {
		t.Fatalf("a different decision was not recorded as different: %+v", result.ShadowComparisons)
	}
	if len(result.Findings) != 1 || result.Findings[0].Code != "shadow_decision_diff" {
		t.Fatalf("findings = %+v, want one shadow_decision_diff", result.Findings)
	}
}

func TestShadowPhaseRefusesAnUnusableCandidateAsAFindingNotAnError(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name      string
		candidate domain.ShadowExecutor
		code      string
		reason    string
	}{
		{"malformed manifest digest", &candidateExecutor{manifest: "sha256:bad"}, "shadow_candidate_invalid", "manifest digest"},
		{"intent outside the catalog", outOfCatalogCandidate{}, "shadow_candidate_invalid", "intent_type_not_allowed"},
		{"executor failure", &candidateExecutor{failWith: errors.New("worker unavailable")}, "shadow_candidate_failed", "worker unavailable"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			result, err := runShadow(t, newRequest(t, alwaysTriggerSpec(t)), &baselineExecutor{}, tc.candidate)
			if err != nil {
				t.Fatalf("a candidate failure aborted the replay: %v", err)
			}
			if len(result.ShadowComparisons) != 0 || !result.WorkerInvoked || !hasFinding(result, tc.code, tc.reason) {
				t.Fatalf("result = %+v, want no comparison and a %s finding mentioning %q", result, tc.code, tc.reason)
			}
		})
	}
}

func TestShadowPhaseFailsWhenTheDeterministicBaselineFails(t *testing.T) {
	t.Parallel()
	failing := errors.New("baseline unavailable")
	_, err := runShadow(t, newRequest(t, alwaysTriggerSpec(t)), &baselineExecutor{failWith: failing}, &candidateExecutor{})
	if !errors.Is(err, failing) || !strings.Contains(err.Error(), "baseline shadow episode") {
		t.Fatalf("a failing baseline = %v, want it wrapped as baseline shadow episode", err)
	}
}

func TestShadowModeRequiresBothExecutors(t *testing.T) {
	t.Parallel()
	for name, capabilities := range map[string]domain.Capabilities{
		"candidate only": {ShadowExecutor: &candidateExecutor{}},
		"baseline only":  {BaselineExecutor: &baselineExecutor{}},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			_, err := RunMode(t.Context(), domain.ModeShadow, newRequest(t, fixtureSpec), capabilities)
			if !errors.Is(err, domain.ErrModeCapabilityRequired) {
				t.Fatalf("shadow mode with %s = %v, want ErrModeCapabilityRequired", name, err)
			}
		})
	}
}

type outOfCatalogCandidate struct{}

func (outOfCatalogCandidate) ExecuteShadow(_ context.Context, input domain.ShadowInput) (domain.ShadowOutput, error) {
	intent := map[string]any{
		"intent_id": "int_attack_" + input.EpisodeID, "decision_id": "dec_attack_" + input.EpisodeID,
		"tenant_id": input.TenantID, "situation_id": input.SituationID, "situation_version": input.SituationVersion,
		"type": "delete_everything", "risk_class": "R1", "parameters": map[string]any{},
		"expires_at": "2099-01-01T00:00:00Z",
	}
	intentDigest, err := contractsv1.IntentDigest(intent)
	if err != nil {
		return domain.ShadowOutput{}, err //nolint:wrapcheck // test double.
	}
	intent["intent_digest"] = intentDigest
	decision := map[string]any{
		"decision_id": "dec_attack_" + input.EpisodeID, "episode_id": input.EpisodeID,
		"attempt_id": input.AttemptID, "fence": input.Fence, "snapshot_digest": input.SnapshotDigest,
		"situation_id": input.SituationID, "situation_version": input.SituationVersion, "confidence": 1.0,
		"intents": []any{intent},
	}
	return shadowOutputOf(decision, "tamoz-attack-v1", "sha256:"+strings.Repeat("b", 64))
}
