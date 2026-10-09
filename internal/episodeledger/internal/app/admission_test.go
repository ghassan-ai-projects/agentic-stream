package app

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/episodeledger/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/episodeledger/internal/store"
)

func TestASituationHasOneLiveEpisodeAndOnlyAReconsiderationReportsTheConflict(t *testing.T) {
	t.Parallel()
	for kind, reportsConflict := range map[string]bool{domain.KindReconsider: true, domain.KindStandard: false} {
		t.Run(kind, func(t *testing.T) {
			t.Parallel()
			within(t, func(ctx context.Context, tx *store.Tx, _ *sql.Tx) {
				first := admission("e1")
				first.SituationID = "same"
				must(t, Admit(ctx, tx, first, now))
				second := admission("e2")
				second.SituationID, second.Kind = "same", kind
				err := Admit(ctx, tx, second, now)
				if err == nil || errors.Is(err, domain.ErrLiveEpisodeConflict) != reportsConflict || store.IsLiveEpisodeViolation(err) != true {
					t.Fatalf("%s collision = %v, want conflict reported = %v with the database violation kept", kind, err, reportsConflict)
				}
			})
		})
	}
}

func TestASituationMayAdmitANewEpisodeOnceItsLiveEpisodeClosed(t *testing.T) {
	t.Parallel()
	within(t, func(ctx context.Context, tx *store.Tx, raw *sql.Tx) {
		first := admission("e1")
		first.SituationID = "same"
		must(t, Admit(ctx, tx, first, now))
		closeEpisode(t, ctx, raw, "e1", domain.LifecycleConcluded)
		second := admission("e2")
		second.SituationID, second.Kind = "same", domain.KindReconsider
		must(t, Admit(ctx, tx, second, now))
	})
}

func TestAnAdmissionOtherThanALiveConflictFailsUnchanged(t *testing.T) {
	t.Parallel()
	within(t, func(ctx context.Context, tx *store.Tx, _ *sql.Tx) {
		admit(t, ctx, tx, "e1")
		duplicate := admission("e1")
		duplicate.Kind, duplicate.SituationID = domain.KindReconsider, "elsewhere"
		if err := Admit(ctx, tx, duplicate, now); err == nil || errors.Is(err, domain.ErrLiveEpisodeConflict) {
			t.Fatalf("a duplicate episode id = %v, want a plain failure that is not the live-episode conflict", err)
		}
	})
}
