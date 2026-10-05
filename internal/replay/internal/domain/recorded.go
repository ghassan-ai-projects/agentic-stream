package domain

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"

	"github.com/ghassan-ai-projects/agentic-stream/internal/canonicaljson"
	"github.com/ghassan-ai-projects/agentic-stream/internal/contractsv1"
)

// RecordedEntry is the immutable worker result recorded with an episode.
// Replay compares by the stable situation/version/trigger key and never calls
// a worker to recreate it.
type RecordedEntry struct {
	EpisodeKey              string
	SituationID             string
	SituationVersion        int
	TriggerID               string
	EpisodeID               string
	AttemptID               string
	Fence                   int64
	AttemptProvenanceSHA256 string
	DecisionJSON            []byte
	DecisionSHA256          string
	ManifestSHA256          string
}

// RecordedLedger supplies worker results from a durable, read-only ledger.
type RecordedLedger interface {
	Entries(context.Context) ([]RecordedEntry, error)
}

// RecordedLedgerForReplay binds recorded entries to the exact replay worklist.
type RecordedLedgerForReplay interface {
	EntriesForReplay(context.Context, []ReplayEpisode) ([]RecordedEntry, error)
}

// IndexRecordedEntries verifies every entry and keys it by episode key,
// rejecting duplicates.
func IndexRecordedEntries(entries []RecordedEntry) (map[string]RecordedEntry, error) {
	byKey := make(map[string]RecordedEntry, len(entries))
	for _, entry := range entries {
		if err := VerifyRecordedEntry(entry); err != nil {
			return nil, err
		}
		if _, exists := byKey[entry.EpisodeKey]; exists {
			return nil, fmt.Errorf("recorded ledger contains duplicate episode key %q", entry.EpisodeKey)
		}
		byKey[entry.EpisodeKey] = entry
	}
	return byKey, nil
}

// VerifyRecordedEntry requires a complete entry whose attempt provenance and
// schema-valid decision both match their digests.
func VerifyRecordedEntry(entry RecordedEntry) error {
	if entry.EpisodeKey == "" || entry.SituationID == "" || entry.SituationVersion <= 0 || entry.TriggerID == "" || entry.EpisodeID == "" || entry.AttemptID == "" || entry.Fence <= 0 || entry.AttemptProvenanceSHA256 == "" || len(entry.DecisionJSON) == 0 || entry.DecisionSHA256 == "" {
		return fmt.Errorf("recorded ledger contains an incomplete entry")
	}
	provenance, err := canonicaljson.Digest(canonicaljson.DomainOutcome, map[string]any{"episode_id": entry.EpisodeID, "attempt_id": entry.AttemptID, "fence": entry.Fence})
	if err != nil || provenance != entry.AttemptProvenanceSHA256 {
		return fmt.Errorf("recorded ledger attempt provenance is invalid for %q", entry.EpisodeKey)
	}
	return VerifyRecordedDocument(entry)
}

// VerifyRecordedDocument requires a canonical, schema-valid decision whose
// digest matches the recorded digest.
func VerifyRecordedDocument(entry RecordedEntry) error {
	canonical, err := canonicaljson.Marshal(json.RawMessage(entry.DecisionJSON))
	if err != nil {
		return fmt.Errorf("recorded ledger decision %q is not canonical JSON: %w", entry.EpisodeKey, err)
	}
	var decision map[string]any
	if err := json.Unmarshal(canonical, &decision); err != nil {
		return fmt.Errorf("recorded ledger decision %q is invalid JSON: %w", entry.EpisodeKey, err)
	}
	if err := contractsv1.Validate(contractsv1.SchemaDecision, decision); err != nil {
		return fmt.Errorf("recorded ledger decision %q violates the decision schema: %w", entry.EpisodeKey, err)
	}
	if !canonicaljson.Verify(canonicaljson.DomainDecision, decision, entry.DecisionSHA256) {
		return fmt.Errorf("recorded ledger decision %q has an invalid digest", entry.EpisodeKey)
	}
	return nil
}

// RequireEmptyRecordedLedger rejects entries when replay produced no episodes.
func RequireEmptyRecordedLedger(entries []RecordedEntry) error {
	if len(entries) > 0 {
		return fmt.Errorf("recorded ledger is non-empty but replay produced no executable episodes")
	}
	return nil
}

// MatchRecordedMetadata requires the entry to describe exactly this episode.
func MatchRecordedMetadata(entry RecordedEntry, episode ReplayEpisode) error {
	if entry.SituationID != episode.SituationID || entry.SituationVersion != episode.SituationVersion || entry.TriggerID != episode.TriggerID || entry.EpisodeID != episode.EpisodeID {
		return fmt.Errorf("recorded ledger metadata does not match replay episode %q", episode.EpisodeKey)
	}
	return nil
}

// DecodeRecordedDecision returns the decision document only when the recorded
// bytes are already canonical JSON.
func DecodeRecordedDecision(entry RecordedEntry) (map[string]any, error) {
	canonical, err := canonicaljson.Marshal(json.RawMessage(entry.DecisionJSON))
	if err != nil {
		return nil, fmt.Errorf("canonicalize recorded decision %q: %w", entry.EpisodeKey, err)
	}
	if !bytes.Equal(canonical, entry.DecisionJSON) {
		return nil, fmt.Errorf("recorded ledger decision %q is not canonical JSON", entry.EpisodeKey)
	}
	var decision map[string]any
	if err := json.Unmarshal(canonical, &decision); err != nil {
		return nil, fmt.Errorf("decode recorded decision %q: %w", entry.EpisodeKey, err)
	}
	return decision, nil
}

// ValidateRecordedSnapshot requires the decision to cite the replayed
// situation version and its persisted snapshot digest.
func ValidateRecordedSnapshot(entry RecordedEntry, episode ReplayEpisode, decision map[string]any, snapshotDigest []byte) error {
	if got, _ := decision["situation_id"].(string); got != episode.SituationID {
		return fmt.Errorf("recorded decision %q has mismatched situation", entry.EpisodeKey)
	}
	if got, ok := decision["situation_version"].(float64); !ok || int(got) != episode.SituationVersion {
		return fmt.Errorf("recorded decision %q has mismatched situation version", entry.EpisodeKey)
	}
	if got, _ := decision["snapshot_digest"].(string); got != "sha256:"+hex.EncodeToString(snapshotDigest) {
		return fmt.Errorf("recorded decision %q has mismatched snapshot digest", entry.EpisodeKey)
	}
	return nil
}

// ValidateRecordedAttempt requires the decision to cite the recorded attempt
// identity and fence: episode first, then attempt, then fence.
func ValidateRecordedAttempt(entry RecordedEntry, decision map[string]any) error {
	if got, _ := decision["episode_id"].(string); got != entry.EpisodeID {
		return fmt.Errorf("recorded decision %q has mismatched episode identity", entry.EpisodeKey)
	}
	if got, _ := decision["attempt_id"].(string); got != entry.AttemptID {
		return fmt.Errorf("recorded decision %q has mismatched attempt identity", entry.EpisodeKey)
	}
	if got, ok := decision["fence"].(float64); !ok || int64(got) != entry.Fence {
		return fmt.Errorf("recorded decision %q has mismatched fence", entry.EpisodeKey)
	}
	return nil
}

// RequireExpectedRecordedKeys rejects ledger decisions outside the worklist.
func RequireExpectedRecordedKeys(episodesByKey map[string]ReplayEpisode, byKey map[string]RecordedEntry) error {
	for key := range byKey {
		if _, ok := episodesByKey[key]; !ok {
			return fmt.Errorf("recorded ledger contains unexpected decision %q", key)
		}
	}
	return nil
}
