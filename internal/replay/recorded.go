package replay

import (
	"context"
	"fmt"

	domain "github.com/ghassan-ai-projects/agentic-stream/internal/replay/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
)

// applyRecorded requires the ledger to hold exactly one valid recorded
// decision per replay episode, each matching the episode it is keyed to.
func applyRecorded(ctx context.Context, db *storage.DB, ledger RecordedLedger, episodes []domain.ReplayEpisode, result *Result) error {
	entries, byKey, err := indexedRecordedEntries(ctx, ledger, episodes)
	if err != nil {
		return err
	}
	if len(episodes) == 0 {
		return domain.RequireEmptyRecordedLedger(entries)
	}
	if err := matchRecordedEpisodes(ctx, db, episodes, byKey); err != nil {
		return err
	}
	result.CapabilityCalls = len(entries)
	return nil
}

func indexedRecordedEntries(ctx context.Context, ledger RecordedLedger, episodes []domain.ReplayEpisode) ([]RecordedEntry, map[string]RecordedEntry, error) {
	entries, err := recordedEntries(ctx, ledger, episodes)
	if err != nil {
		return nil, nil, fmt.Errorf("read recorded ledger: %w", err)
	}
	byKey, err := domain.IndexRecordedEntries(entries)
	if err != nil {
		return nil, nil, err
	}
	return entries, byKey, nil
}

func recordedEntries(ctx context.Context, ledger RecordedLedger, episodes []domain.ReplayEpisode) ([]RecordedEntry, error) {
	if stronger, ok := ledger.(RecordedLedgerForReplay); ok {
		view := domain.EpisodeViews(episodes)
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

func matchRecordedEpisodes(ctx context.Context, db *storage.DB, episodes []domain.ReplayEpisode, byKey map[string]RecordedEntry) error {
	episodesByKey := make(map[string]domain.ReplayEpisode, len(episodes))
	for _, episode := range episodes {
		episodesByKey[episode.EpisodeKey] = episode
		if err := matchRecordedEpisode(ctx, db, episode, byKey); err != nil {
			return err
		}
	}
	return domain.RequireExpectedRecordedKeys(episodesByKey, byKey)
}

func matchRecordedEpisode(ctx context.Context, db *storage.DB, episode domain.ReplayEpisode, byKey map[string]RecordedEntry) error {
	entry, ok := byKey[episode.EpisodeKey]
	if !ok {
		return fmt.Errorf("recorded ledger is missing decision %q", episode.EpisodeKey)
	}
	if err := domain.MatchRecordedMetadata(entry, episode); err != nil {
		return err
	}
	return validateRecordedDecision(ctx, db, entry, episode)
}

func validateRecordedDecision(ctx context.Context, db *storage.DB, entry RecordedEntry, episode domain.ReplayEpisode) error {
	var snapshotDigest []byte
	if err := db.QueryRowContext(ctx, `SELECT snapshot_sha256 FROM situation_versions WHERE situation_id = ? AND version = ?`, episode.SituationID, episode.SituationVersion).Scan(&snapshotDigest); err != nil {
		return fmt.Errorf("load recorded snapshot digest: %w", err)
	}
	decision, err := domain.DecodeRecordedDecision(entry)
	if err != nil {
		return err
	}
	if err := domain.ValidateRecordedSnapshot(entry, episode, decision, snapshotDigest); err != nil {
		return err
	}
	return domain.ValidateRecordedAttempt(entry, decision)
}
