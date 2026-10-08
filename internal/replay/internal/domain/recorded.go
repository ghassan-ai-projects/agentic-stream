package domain

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"

	"github.com/ghassan-ai-projects/agentic-stream/internal/canonicaljson"
	"github.com/ghassan-ai-projects/agentic-stream/internal/contractsv1"
)

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

type RecordedLedger interface {
	Entries(context.Context) ([]RecordedEntry, error)
}

type RecordedLedgerForReplay interface {
	EntriesForReplay(context.Context, []ReplayEpisode) ([]RecordedEntry, error)
}

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

func VerifyRecordedEntry(entry RecordedEntry) error {
	if entry.EpisodeKey == "" || entry.SituationID == "" || entry.SituationVersion <= 0 || entry.TriggerID == "" || entry.EpisodeID == "" || entry.AttemptID == "" || entry.Fence <= 0 || entry.AttemptProvenanceSHA256 == "" || len(entry.DecisionJSON) == 0 || entry.DecisionSHA256 == "" {
		return fmt.Errorf("recorded ledger contains an incomplete entry")
	}
	provenance, err := AttemptProvenance(entry)
	if err != nil || provenance != entry.AttemptProvenanceSHA256 {
		return fmt.Errorf("recorded ledger attempt provenance is invalid for %q", entry.EpisodeKey)
	}
	return VerifyRecordedDocument(entry)
}

func AttemptProvenance(entry RecordedEntry) (string, error) {
	provenance, err := canonicaljson.Digest(canonicaljson.DomainOutcome, map[string]any{"episode_id": entry.EpisodeID, "attempt_id": entry.AttemptID, "fence": entry.Fence})
	if err != nil {
		return "", fmt.Errorf("digest attempt provenance: %w", err)
	}
	return provenance, nil
}

func VerifyRecordedDocument(entry RecordedEntry) error {
	sum, err := canonicaljson.DecodeDigest(entry.DecisionSHA256)
	if err != nil {
		return fmt.Errorf("recorded ledger decision %q has an invalid digest: %w", entry.EpisodeKey, err)
	}
	if _, err := contractsv1.VerifyStoredDocument(contractsv1.SchemaDecision, canonicaljson.DomainDecision, entry.DecisionJSON, sum); err != nil {
		return fmt.Errorf("recorded ledger decision %q: %w", entry.EpisodeKey, err)
	}
	return nil
}

func RequireEmptyRecordedLedger(entries []RecordedEntry) error {
	if len(entries) > 0 {
		return fmt.Errorf("recorded ledger is non-empty but replay produced no executable episodes")
	}
	return nil
}

func MatchRecordedMetadata(entry RecordedEntry, episode ReplayEpisode) error {
	if entry.SituationID != episode.SituationID || entry.SituationVersion != episode.SituationVersion || entry.TriggerID != episode.TriggerID {
		return fmt.Errorf("recorded ledger metadata does not match replay episode %q", episode.EpisodeKey)
	}
	return nil
}

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

func RecordedCitation(entry RecordedEntry, episode ReplayEpisode, decision map[string]any) (int, error) {
	if got, _ := decision["situation_id"].(string); got != episode.SituationID {
		return 0, fmt.Errorf("recorded decision %q has mismatched situation", entry.EpisodeKey)
	}
	cited, ok := decision["situation_version"].(float64)
	if !ok || int(cited) < episode.SituationVersion {
		return 0, fmt.Errorf("recorded decision %q has mismatched situation version", entry.EpisodeKey)
	}
	return int(cited), nil
}

func ValidateRecordedSnapshot(entry RecordedEntry, decision map[string]any, snapshotDigest []byte) error {
	if got, _ := decision["snapshot_digest"].(string); got != canonicaljson.EncodeDigest(snapshotDigest) {
		return fmt.Errorf("recorded decision %q has mismatched snapshot digest", entry.EpisodeKey)
	}
	return nil
}

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

func RequireExpectedRecordedKeys(episodesByKey map[string]ReplayEpisode, byKey map[string]RecordedEntry) error {
	for key := range byKey {
		if _, ok := episodesByKey[key]; !ok {
			return fmt.Errorf("recorded ledger contains unexpected decision %q", key)
		}
	}
	return nil
}
