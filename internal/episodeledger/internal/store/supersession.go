package store

import (
	"context"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/kernel"
)

// SupersedeCoalescedEpisodes ends the live episodes bound to coalesced
// scheduler items of the situation.
func (t *Tx) SupersedeCoalescedEpisodes(ctx context.Context, situationID string, now time.Time) error {
	return t.exec(ctx, "supersede episodes", supersedeCoalescedEpisodesSQL, kernel.FormatTime(now), situationID)
}

// CancelCoalescedAttempts asks the in-flight attempts of superseded, coalesced
// episodes to cancel.
func (t *Tx) CancelCoalescedAttempts(ctx context.Context, situationID string) error {
	return t.exec(ctx, "cancel superseded attempts", cancelCoalescedAttemptsSQL, situationID)
}

var supersedeCoalescedEpisodesSQL = `
		UPDATE episodes SET lifecycle_status = 'superseded', ended_at = ?
		WHERE scheduler_item_id IN (
			SELECT scheduler_item_id FROM scheduler_items
			WHERE situation_id = ? AND status = 'coalesced'
		) AND lifecycle_status IN ` + liveLifecycles

var cancelCoalescedAttemptsSQL = `
		UPDATE episode_attempts SET status = 'cancelling'
		WHERE episode_id IN (
			SELECT episode_id FROM episodes
			WHERE scheduler_item_id IN (
				SELECT scheduler_item_id FROM scheduler_items
				WHERE situation_id = ? AND status = 'coalesced'
			) AND lifecycle_status = 'superseded'
		) AND status IN ` + inFlightAttempts
