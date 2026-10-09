package app

import (
	"context"
	"database/sql"
	"strings"
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/episodeledger/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/episodeledger/internal/store"
)

func TestARejectionOfAKnownEpisodeIsAuditedWithItsAttemptAndFence(t *testing.T) {
	t.Parallel()
	within(t, func(ctx context.Context, tx *store.Tx, raw *sql.Tx) {
		admit(t, ctx, tx, "e1")
		identity := beginAttempt(t, ctx, tx, "e1", "a1")
		must(t, RecordRejection(ctx, tx, identity, domain.RejectSchemaInvalid, []byte(`{"path":"$.intents[0]"}`), now))
		row := queryText(t, ctx, raw, "SELECT episode_id || '|' || attempt_id || '|' || fence || '|' || reason || '|' || CAST(details_json AS TEXT) || '|' || created_at FROM episode_rejections")
		if want := `e1|a1|1|schema_invalid|{"path":"$.intents[0]"}|` + ts(0); row != want {
			t.Fatalf("rejection row = %s, want %s", row, want)
		}
	})
}

func TestARejectionOfAnUnknownEpisodeIsAuditedWithoutAForeignKey(t *testing.T) {
	t.Parallel()
	within(t, func(ctx context.Context, tx *store.Tx, raw *sql.Tx) {
		forged := domain.Identity{EpisodeID: "missing", AttemptID: "a", Fence: 1}
		must(t, RecordRejection(ctx, tx, forged, domain.RejectUnknownEpisode, nil, now))
		must(t, RecordRejection(ctx, tx, domain.Identity{AttemptID: "a"}, domain.RejectUnknownEpisode, nil, now.Add(time.Second)))
		if got, want := queryText(t, ctx, raw, "SELECT COUNT(*) || '|' || COUNT(episode_id) || '|' || MIN(CAST(details_json AS TEXT)) FROM episode_rejections"), "2|0|{}"; got != want {
			t.Fatalf("rejections = %s, want %s: two rows, no episode reference, details defaulted", got, want)
		}
	})
}

func TestRepeatingARejectionChangesNothingAndANewInstantIsANewRejection(t *testing.T) {
	t.Parallel()
	within(t, func(ctx context.Context, tx *store.Tx, raw *sql.Tx) {
		admit(t, ctx, tx, "e1")
		identity := domain.Identity{EpisodeID: "e1", AttemptID: "a1", Fence: 1}
		for range 3 {
			must(t, RecordRejection(ctx, tx, identity, domain.RejectStaleAttempt, []byte(`{"same":true}`), now))
		}
		if count := queryText(t, ctx, raw, "SELECT COUNT(*) FROM episode_rejections"); count != "1" {
			t.Fatalf("rejections after repeating = %s, want 1", count)
		}
		must(t, RecordRejection(ctx, tx, identity, domain.RejectStaleAttempt, []byte(`{"same":true}`), now.Add(time.Nanosecond)))
		must(t, RecordRejection(ctx, tx, identity, domain.RejectWrongAttempt, []byte(`{"same":true}`), now))
		if count := queryText(t, ctx, raw, "SELECT COUNT(*) FROM episode_rejections"); count != "3" {
			t.Fatalf("rejections after a new instant and a new reason = %s, want 3", count)
		}
	})
}

func TestAnUnregisteredRejectionReasonIsRefusedBeforeAnythingIsWritten(t *testing.T) {
	t.Parallel()
	within(t, func(ctx context.Context, tx *store.Tx, raw *sql.Tx) {
		err := RecordRejection(ctx, tx, domain.Identity{EpisodeID: "e1"}, "made_up", nil, now)
		if err == nil || !strings.Contains(err.Error(), `invalid rejection reason "made_up"`) {
			t.Fatalf("RecordRejection(made_up) = %v, want the reason refused by name", err)
		}
		if count := queryText(t, ctx, raw, "SELECT COUNT(*) FROM episode_rejections"); count != "0" {
			t.Fatalf("a refused reason left %s rows", count)
		}
	})
}
