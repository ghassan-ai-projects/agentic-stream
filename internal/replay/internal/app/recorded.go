package app

import (
	"context"
	"fmt"
	"time"

	domain "github.com/ghassan-ai-projects/agentic-stream/internal/replay/internal/domain"
)

// applyCapabilities runs the worker-aware part of a replay mode: recorded
// decisions are verified against the replay worklist and shadow executors are
// compared report-only.
func applyCapabilities(ctx context.Context, session *replaySession, mode domain.Mode, caps domain.Capabilities, evaluationTime time.Time, result *domain.Result) error {
	episodes, err := session.store.EpisodeWorklist(ctx, session.tenantID)
	if err != nil {
		return err
	}
	switch mode {
	case domain.ModeRecorded:
		return applyRecorded(ctx, session, caps.RecordedLedger, episodes, result)
	case domain.ModeShadow:
		return applyPairedShadow(ctx, session, caps, episodes, evaluationTime, result)
	}
	return nil
}

// applyRecorded requires the ledger to hold exactly one valid recorded
// decision per replay episode, each matching the episode it is keyed to.
func applyRecorded(ctx context.Context, session *replaySession, ledger domain.RecordedLedger, episodes []domain.ReplayEpisode, result *domain.Result) error {
	entries, byKey, err := indexedRecordedEntries(ctx, ledger, episodes)
	if err != nil {
		return err
	}
	if len(episodes) == 0 {
		return domain.RequireEmptyRecordedLedger(entries)
	}
	if err := matchRecordedEpisodes(ctx, session, episodes, byKey); err != nil {
		return err
	}
	result.CapabilityCalls = len(entries)
	return nil
}

func indexedRecordedEntries(ctx context.Context, ledger domain.RecordedLedger, episodes []domain.ReplayEpisode) ([]domain.RecordedEntry, map[string]domain.RecordedEntry, error) {
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

func recordedEntries(ctx context.Context, ledger domain.RecordedLedger, episodes []domain.ReplayEpisode) ([]domain.RecordedEntry, error) {
	if stronger, ok := ledger.(domain.RecordedLedgerForReplay); ok {
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

func matchRecordedEpisodes(ctx context.Context, session *replaySession, episodes []domain.ReplayEpisode, byKey map[string]domain.RecordedEntry) error {
	episodesByKey := make(map[string]domain.ReplayEpisode, len(episodes))
	for _, episode := range episodes {
		episodesByKey[episode.EpisodeKey] = episode
		if err := matchRecordedEpisode(ctx, session, episode, byKey); err != nil {
			return err
		}
	}
	return domain.RequireExpectedRecordedKeys(episodesByKey, byKey)
}

func matchRecordedEpisode(ctx context.Context, session *replaySession, episode domain.ReplayEpisode, byKey map[string]domain.RecordedEntry) error {
	entry, ok := byKey[episode.EpisodeKey]
	if !ok {
		return fmt.Errorf("recorded ledger is missing decision %q", episode.EpisodeKey)
	}
	if err := domain.MatchRecordedMetadata(entry, episode); err != nil {
		return err
	}
	return validateRecordedDecision(ctx, session, entry, episode)
}

func validateRecordedDecision(ctx context.Context, session *replaySession, entry domain.RecordedEntry, episode domain.ReplayEpisode) error {
	snapshotDigest, err := session.store.RecordedSnapshotDigest(ctx, episode)
	if err != nil {
		return err
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
