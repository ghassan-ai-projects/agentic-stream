package replay

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/ghassan-ai-projects/agentic-stream/internal/canonicaljson"
	"github.com/ghassan-ai-projects/agentic-stream/internal/contractsv1"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
)

// applyRecorded requires the ledger to hold exactly one valid recorded
// decision per replay episode, each matching the episode it is keyed to.
func applyRecorded(ctx context.Context, db *storage.DB, ledger RecordedLedger, items []replayItem, result *Result) error {
	entries, byKey, err := indexedRecordedEntries(ctx, ledger, items)
	if err != nil {
		return err
	}
	if len(items) == 0 {
		return requireEmptyRecordedLedger(entries)
	}
	if err := matchRecordedEpisodes(ctx, db, items, byKey); err != nil {
		return err
	}
	result.CapabilityCalls = len(entries)
	return nil
}

func indexedRecordedEntries(ctx context.Context, ledger RecordedLedger, items []replayItem) ([]RecordedEntry, map[string]RecordedEntry, error) {
	entries, err := recordedEntries(ctx, ledger, items)
	if err != nil {
		return nil, nil, fmt.Errorf("read recorded ledger: %w", err)
	}
	byKey, err := indexRecordedEntries(entries)
	if err != nil {
		return nil, nil, err
	}
	return entries, byKey, nil
}

func recordedEntries(ctx context.Context, ledger RecordedLedger, items []replayItem) ([]RecordedEntry, error) {
	if stronger, ok := ledger.(RecordedLedgerForReplay); ok {
		view := replayEpisodeViews(items)
		entries, err := stronger.EntriesForReplay(ctx, view)
		if err != nil {
			return nil, fmt.Errorf("entries for replay: %w", err)
		}
		return entries, nil
	}
	entries, err := ledger.Entries(ctx)
	if err != nil {
		return nil, fmt.Errorf("recorded ledger entries: %w", err)
	}
	return entries, nil
}

func replayEpisodeViews(items []replayItem) []ReplayEpisode {
	view := make([]ReplayEpisode, 0, len(items))
	for _, item := range items {
		view = append(view, ReplayEpisode{
			EpisodeKey: replayEpisodeKey(item.SituationID, item.SituationVersion, item.TriggerID),
			EpisodeID:  item.EpisodeID, SituationID: item.SituationID,
			SituationVersion: item.SituationVersion, TriggerID: item.TriggerID,
			SnapshotDigest: item.SnapshotDigest,
		})
	}
	return view
}

// indexRecordedEntries verifies every entry and keys it by episode key,
// rejecting duplicates.
func indexRecordedEntries(entries []RecordedEntry) (map[string]RecordedEntry, error) {
	byKey := make(map[string]RecordedEntry, len(entries))
	for _, entry := range entries {
		if err := verifyRecordedEntry(entry); err != nil {
			return nil, err
		}
		if _, exists := byKey[entry.EpisodeKey]; exists {
			return nil, fmt.Errorf("recorded ledger contains duplicate episode key %q", entry.EpisodeKey)
		}
		byKey[entry.EpisodeKey] = entry
	}
	return byKey, nil
}

// verifyRecordedEntry requires a complete entry whose attempt provenance and
// schema-valid decision both match their digests.
func verifyRecordedEntry(entry RecordedEntry) error {
	if entry.EpisodeKey == "" || entry.SituationID == "" || entry.SituationVersion <= 0 || entry.TriggerID == "" || entry.EpisodeID == "" || entry.AttemptID == "" || entry.Fence <= 0 || entry.AttemptProvenanceSHA256 == "" || len(entry.DecisionJSON) == 0 || entry.DecisionSHA256 == "" {
		return fmt.Errorf("recorded ledger contains an incomplete entry")
	}
	provenance, err := canonicaljson.Digest(canonicaljson.DomainOutcome, map[string]any{"episode_id": entry.EpisodeID, "attempt_id": entry.AttemptID, "fence": entry.Fence})
	if err != nil || provenance != entry.AttemptProvenanceSHA256 {
		return fmt.Errorf("recorded ledger attempt provenance is invalid for %q", entry.EpisodeKey)
	}
	return verifyRecordedDocument(entry)
}

func verifyRecordedDocument(entry RecordedEntry) error {
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

func requireEmptyRecordedLedger(entries []RecordedEntry) error {
	if len(entries) > 0 {
		return fmt.Errorf("recorded ledger is non-empty but replay produced no executable episodes")
	}
	return nil
}

func matchRecordedEpisodes(ctx context.Context, db *storage.DB, items []replayItem, byKey map[string]RecordedEntry) error {
	itemsByKey := make(map[string]replayItem, len(items))
	for _, item := range items {
		key := replayEpisodeKey(item.SituationID, item.SituationVersion, item.TriggerID)
		itemsByKey[key] = item
		if err := matchRecordedEpisode(ctx, db, item, key, byKey); err != nil {
			return err
		}
	}
	return requireExpectedRecordedKeys(itemsByKey, byKey)
}

func matchRecordedEpisode(ctx context.Context, db *storage.DB, item replayItem, key string, byKey map[string]RecordedEntry) error {
	entry, ok := byKey[key]
	if !ok {
		return fmt.Errorf("recorded ledger is missing decision %q", key)
	}
	if entry.SituationID != item.SituationID || entry.SituationVersion != item.SituationVersion || entry.TriggerID != item.TriggerID || entry.EpisodeID != item.EpisodeID {
		return fmt.Errorf("recorded ledger metadata does not match replay episode %q", key)
	}
	if err := validateRecordedDecision(ctx, db, entry, item); err != nil {
		return err
	}
	return nil
}

func validateRecordedDecision(ctx context.Context, db *storage.DB, entry RecordedEntry, item replayItem) error {
	var snapshotDigest []byte
	if err := db.QueryRowContext(ctx, `SELECT snapshot_sha256 FROM situation_versions WHERE situation_id = ? AND version = ?`, item.SituationID, item.SituationVersion).Scan(&snapshotDigest); err != nil {
		return fmt.Errorf("load recorded snapshot digest: %w", err)
	}
	decision, err := decodeRecordedDecision(entry)
	if err != nil {
		return err
	}
	if err := validateRecordedSnapshot(entry, item, decision, snapshotDigest); err != nil {
		return err
	}
	return validateRecordedAttempt(entry, decision)
}

func decodeRecordedDecision(entry RecordedEntry) (map[string]any, error) {
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

func validateRecordedSnapshot(entry RecordedEntry, item replayItem, decision map[string]any, snapshotDigest []byte) error {
	if got, _ := decision["situation_id"].(string); got != item.SituationID {
		return fmt.Errorf("recorded decision %q has mismatched situation", entry.EpisodeKey)
	}
	if got, ok := decision["situation_version"].(float64); !ok || int(got) != item.SituationVersion {
		return fmt.Errorf("recorded decision %q has mismatched situation version", entry.EpisodeKey)
	}
	if got, _ := decision["snapshot_digest"].(string); got != "sha256:"+hex.EncodeToString(snapshotDigest) {
		return fmt.Errorf("recorded decision %q has mismatched snapshot digest", entry.EpisodeKey)
	}
	return nil
}

func validateRecordedAttempt(entry RecordedEntry, decision map[string]any) error {
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

func requireExpectedRecordedKeys(itemsByKey map[string]replayItem, byKey map[string]RecordedEntry) error {
	for key := range byKey {
		if _, ok := itemsByKey[key]; !ok {
			return fmt.Errorf("recorded ledger contains unexpected decision %q", key)
		}
	}
	return nil
}
