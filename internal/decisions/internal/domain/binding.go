package domain

import "github.com/ghassan-ai-projects/agentic-stream/internal/contractsv1"

// checkDecisionBinding requires the Decision to name the dispatched episode,
// attempt, fence, snapshot, and Situation version, and to be unexpired. It
// returns the decision ID.
func checkDecisionBinding(document map[string]any, input Input) (string, error) {
	decisionID, ok := document["decision_id"].(string)
	if !ok || decisionID == "" {
		return "", reject("schema_invalid", "decision_id", "decision_id is required")
	}
	if err := checkDecisionAttempt(document, input); err != nil {
		return "", err
	}
	if err := checkDecisionSnapshot(document, input); err != nil {
		return "", err
	}
	return decisionID, nil
}

func checkDecisionAttempt(document map[string]any, input Input) error {
	if got, _ := document["episode_id"].(string); got != input.EpisodeID {
		return reject("snapshot_mismatch", "episode_id", "decision episode does not match the dispatched episode")
	}
	if got, _ := document["attempt_id"].(string); got != input.AttemptID {
		return reject("stale_attempt", "attempt_id", "decision attempt does not match the dispatched attempt")
	}
	if got, ok := integerField(document, "fence"); !ok || int64(got) != input.Fence {
		return reject("stale_attempt", "fence", "decision fence does not match the dispatched attempt")
	}
	return nil
}

func checkDecisionSnapshot(document map[string]any, input Input) error {
	if got, _ := document["snapshot_digest"].(string); got != input.SnapshotDigest {
		return reject("snapshot_mismatch", "snapshot_digest", "decision snapshot does not match the dispatched snapshot")
	}
	if got, _ := document["situation_id"].(string); got != input.SituationID {
		return reject("snapshot_mismatch", "situation_id", "decision Situation does not match the dispatched Situation")
	}
	if got, ok := integerField(document, "situation_version"); !ok || got != input.SituationVersion {
		return reject("snapshot_mismatch", "situation_version", "decision Situation version does not match the dispatched version")
	}
	if validUntil, ok := document["valid_until"].(string); ok && isExpired(input.Now, validUntil) {
		return reject("expired", "valid_until", "decision validity has expired")
	}
	return nil
}

// decisionIntents returns the intents array. An empty array must explicitly
// request more evidence, and (P4) at most ONE intent may be actionable
// (neither a watch nor a compensation); the v1 contract is enforced here
// independently of the worker's builder.
func decisionIntents(document map[string]any) ([]any, error) {
	rawIntents, ok := document["intents"].([]any)
	if !ok {
		return nil, reject("schema_invalid", "intents", "intents must be an array")
	}
	if err := checkDecisionIntentSet(document, rawIntents); err != nil {
		return nil, err
	}
	return rawIntents, nil
}

func checkDecisionIntentSet(document map[string]any, rawIntents []any) error {
	if len(rawIntents) == 0 {
		if decisionType, _ := document["decision_type"].(string); decisionType != "need_more_evidence" {
			return reject("schema_invalid", "decision_type", "an empty-intent decision must explicitly request more evidence")
		}
	}
	actionable, err := countActionableIntents(rawIntents)
	if err != nil {
		return err
	}
	if actionable > 1 {
		return reject("schema_invalid", "intents",
			"a decision may carry at most one actionable intent")
	}
	return nil
}

func countActionableIntents(rawIntents []any) (int, error) {
	actionable := 0
	for _, rawIntent := range rawIntents {
		intent, ok := rawIntent.(map[string]any)
		if !ok {
			return 0, reject("schema_invalid", "intents", "intent must be an object")
		}
		intentType, _ := intent["type"].(string)
		if intentType == "install_watch_condition" || contractsv1.DocumentString(intent, "compensates") != "" {
			continue
		}
		actionable++
	}
	return actionable, nil
}
