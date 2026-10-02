package replay

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/canonicaljson"
	"github.com/ghassan-ai-projects/agentic-stream/internal/contractsv1"
	"github.com/ghassan-ai-projects/agentic-stream/internal/spec"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
)

// applyCapabilities runs the worker-aware part of a replay mode: recorded
// decisions are verified against the replay worklist, shadow executors are
// compared report-only, and counterfactual commands go only to the simulator.
func applyCapabilities(ctx context.Context, db *storage.DB, tenantID string, mode Mode, caps Capabilities, compiled *spec.CompiledSpec, evaluationTime time.Time, result *Result) error {
	items, err := loadReplayItems(ctx, db, tenantID)
	if err != nil {
		return err
	}
	switch mode {
	case ModeRecorded:
		return applyRecorded(ctx, db, caps.RecordedLedger, items, result)
	case ModeShadow:
		return applyPairedShadow(ctx, db, tenantID, caps, compiled, items, evaluationTime, result)
	case ModeCounterfactual:
		return applyCounterfactual(ctx, caps, result)
	}
	return nil
}

// applyRecorded requires the ledger to hold exactly one valid recorded
// decision per replay episode, each matching the episode it is keyed to.
func applyRecorded(ctx context.Context, db *storage.DB, ledger RecordedLedger, items []replayItem, result *Result) error {
	entries, err := recordedEntries(ctx, ledger, items)
	if err != nil {
		return fmt.Errorf("read recorded ledger: %w", err)
	}
	byKey, err := indexRecordedEntries(entries)
	if err != nil {
		return err
	}
	if len(items) == 0 {
		if len(entries) > 0 {
			return fmt.Errorf("recorded ledger is non-empty but replay produced no executable episodes")
		}
		return nil
	}
	if err := matchRecordedEpisodes(ctx, db, items, byKey); err != nil {
		return err
	}
	result.CapabilityCalls = len(entries)
	return nil
}

func matchRecordedEpisodes(ctx context.Context, db *storage.DB, items []replayItem, byKey map[string]RecordedEntry) error {
	itemsByKey := make(map[string]replayItem, len(items))
	for _, item := range items {
		key := replayEpisodeKey(item.SituationID, item.SituationVersion, item.TriggerID)
		itemsByKey[key] = item
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
	}
	for key := range byKey {
		if _, ok := itemsByKey[key]; !ok {
			return fmt.Errorf("recorded ledger contains unexpected decision %q", key)
		}
	}
	return nil
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

// applyCounterfactual sends each distinct, complete counterfactual command to
// the simulator only. Replay never reaches an effector.
func applyCounterfactual(ctx context.Context, caps Capabilities, result *Result) error {
	if len(caps.Commands) == 0 {
		return fmt.Errorf("counterfactual command set is empty")
	}
	seenCommands := make(map[string]struct{}, len(caps.Commands))
	for _, command := range caps.Commands {
		if command.CommandID == "" || command.Route == "" || command.Target == "" {
			return fmt.Errorf("counterfactual command is incomplete")
		}
		if _, exists := seenCommands[command.CommandID]; exists {
			return fmt.Errorf("counterfactual command %q is duplicated", command.CommandID)
		}
		seenCommands[command.CommandID] = struct{}{}
		output, err := caps.Simulator.Simulate(ctx, command)
		if err != nil {
			return fmt.Errorf("simulate command %s: %w", command.CommandID, err)
		}
		if output == nil {
			return fmt.Errorf("simulator returned no outcome for command %s", command.CommandID)
		}
		result.SimulatedResults = append(result.SimulatedResults, output)
		result.CapabilityCalls++
	}
	return nil
}

func recordedEntries(ctx context.Context, ledger RecordedLedger, items []replayItem) ([]RecordedEntry, error) {
	if stronger, ok := ledger.(RecordedLedgerForReplay); ok {
		view := make([]ReplayEpisode, 0, len(items))
		for _, item := range items {
			view = append(view, ReplayEpisode{
				EpisodeKey: replayEpisodeKey(item.SituationID, item.SituationVersion, item.TriggerID),
				EpisodeID:  item.EpisodeID, SituationID: item.SituationID,
				SituationVersion: item.SituationVersion, TriggerID: item.TriggerID,
				SnapshotDigest: item.SnapshotDigest,
			})
		}
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

type replayItem struct {
	TriggerID        string
	SituationID      string
	SituationVersion int
	EpisodeID        string
	SnapshotDigest   string
}

func loadReplayItems(ctx context.Context, db *storage.DB, tenantID string) ([]replayItem, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT DISTINCT si.trigger_id, si.situation_id, si.situation_version, e.episode_id, sv.snapshot_sha256
		FROM scheduler_items si
		JOIN episodes e ON e.scheduler_item_id = si.scheduler_item_id
		JOIN situation_versions sv ON sv.situation_id = si.situation_id AND sv.version = si.situation_version
		WHERE si.tenant_id = ?
		ORDER BY si.situation_id, si.situation_version, si.trigger_id`, tenantID)
	if err != nil {
		return nil, fmt.Errorf("query replay items: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var items []replayItem
	for rows.Next() {
		var item replayItem
		var snapshotDigest []byte
		if err := rows.Scan(&item.TriggerID, &item.SituationID, &item.SituationVersion, &item.EpisodeID, &snapshotDigest); err != nil {
			return nil, fmt.Errorf("scan replay item: %w", err)
		}
		item.SnapshotDigest = "sha256:" + hex.EncodeToString(snapshotDigest)
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate replay items: %w", err)
	}
	return items, nil
}

func validateRecordedDecision(ctx context.Context, db *storage.DB, entry RecordedEntry, item replayItem) error {
	var snapshotDigest []byte
	if err := db.QueryRowContext(ctx, `SELECT snapshot_sha256 FROM situation_versions WHERE situation_id = ? AND version = ?`, item.SituationID, item.SituationVersion).Scan(&snapshotDigest); err != nil {
		return fmt.Errorf("load recorded snapshot digest: %w", err)
	}
	canonical, err := canonicaljson.Marshal(json.RawMessage(entry.DecisionJSON))
	if err != nil {
		return fmt.Errorf("canonicalize recorded decision %q: %w", entry.EpisodeKey, err)
	}
	if !bytes.Equal(canonical, entry.DecisionJSON) {
		return fmt.Errorf("recorded ledger decision %q is not canonical JSON", entry.EpisodeKey)
	}
	var decision map[string]any
	if err := json.Unmarshal(canonical, &decision); err != nil {
		return fmt.Errorf("decode recorded decision %q: %w", entry.EpisodeKey, err)
	}
	if got, _ := decision["situation_id"].(string); got != item.SituationID {
		return fmt.Errorf("recorded decision %q has mismatched situation", entry.EpisodeKey)
	}
	if got, ok := decision["situation_version"].(float64); !ok || int(got) != item.SituationVersion {
		return fmt.Errorf("recorded decision %q has mismatched situation version", entry.EpisodeKey)
	}
	if got, _ := decision["snapshot_digest"].(string); got != "sha256:"+hex.EncodeToString(snapshotDigest) {
		return fmt.Errorf("recorded decision %q has mismatched snapshot digest", entry.EpisodeKey)
	}
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
