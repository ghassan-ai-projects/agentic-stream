package domain

import "fmt"

// ReplayEpisode is one executable episode of the replay worklist and the
// trusted projection identity supplied to a recorded ledger. The episode key
// is the stable situation/version/trigger identity shared by the worklist,
// recorded ledgers and shadow comparisons.
type ReplayEpisode struct {
	EpisodeKey       string
	EpisodeID        string
	SituationID      string
	SituationVersion int
	TriggerID        string
	SnapshotDigest   string
}

// EpisodeKey builds the stable situation/version/trigger identity.
func EpisodeKey(situationID string, version int, triggerID string) string {
	return fmt.Sprintf("%s/%d/%s", situationID, version, triggerID)
}

// Keyed returns the episode with its derived episode key filled in.
func (e ReplayEpisode) Keyed() ReplayEpisode {
	e.EpisodeKey = EpisodeKey(e.SituationID, e.SituationVersion, e.TriggerID)
	return e
}

// EpisodeViews projects the worklist as trusted ledger identities.
func EpisodeViews(episodes []ReplayEpisode) []ReplayEpisode {
	view := make([]ReplayEpisode, 0, len(episodes))
	for _, episode := range episodes {
		view = append(view, episode.Keyed())
	}
	return view
}
