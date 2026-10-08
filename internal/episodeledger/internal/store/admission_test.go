package store_test

import (
	"context"
	"database/sql"
	"reflect"
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/episodeledger/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/episodeledger/internal/store"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage/storagetest"
)

func fullAdmission(t *testing.T) domain.Admission {
	t.Helper()
	admission := domain.Admission{}
	value := reflect.ValueOf(&admission).Elem()
	for i := range value.NumField() {
		field := value.Field(i)
		switch field.Kind() {
		case reflect.String:
			field.SetString(value.Type().Field(i).Name)
		case reflect.Int:
			field.SetInt(int64(i + 1))
		case reflect.Slice:
			field.SetBytes(append(make([]byte, 31), byte(i+1)))
		default:
			t.Fatalf("admission field %s has unhandled kind %s", value.Type().Field(i).Name, field.Kind())
		}
	}
	admission.Kind = ""
	admission.DispatchPolicy = "active"
	return admission
}

func TestAdmissionColumnsRoundTripEveryField(t *testing.T) {
	t.Parallel()
	within(t, func(ctx context.Context, tx *store.Tx) {
		want := fullAdmission(t)
		if err := tx.InsertEpisode(ctx, want, time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)); err != nil {
			t.Fatal(err)
		}
		got, err := tx.ReadAdmission(ctx, want.EpisodeID)
		if err != nil || !reflect.DeepEqual(got, want) {
			t.Fatalf("admission = %+v err = %v, want %+v", got, err, want)
		}
		if _, err := tx.ReadAdmission(ctx, "missing"); err == nil {
			t.Fatal("unknown episode read")
		}
	})
}

func TestNextDispatchableEpisodeOrdersLiveEpisodesAndAdmitsKilledUnstarted(t *testing.T) {
	t.Parallel()
	db := storagetest.OpenTempWithoutForeignKeys(t)
	err := db.WithTx(t.Context(), func(raw *sql.Tx) error {
		ctx, tx := t.Context(), store.Join(raw)
		for id, acceptedAt := range map[string]time.Time{"e-late": time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC), "e-dead": time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)} {
			if err := tx.InsertEpisode(ctx, admitted(id), acceptedAt); err != nil {
				return err
			}
		}
		for _, query := range []string{
			`UPDATE episodes SET lifecycle_status = 'superseded', policy_epoch = 'dead' WHERE episode_id = 'e-dead'`,
			`INSERT INTO epoch_control (epoch, state, updated_at) VALUES ('dead', 'killed', '2026-01-01T00:00:00Z')`,
			`UPDATE episodes SET stale_rebind_count = 2 WHERE episode_id = 'e-late'`,
		} {
			if _, err := raw.ExecContext(ctx, query); err != nil {
				return err
			}
		}
		for _, tc := range []struct {
			name, tenant, want string
			killed             bool
			rebinds            int
		}{
			{"live only", "t", "e-late", false, 2},
			{"killed unstarted first", "t", "e-dead", true, 0},
			{"other tenant", "other", "", true, 0},
		} {
			got, found, err := tx.NextDispatchableEpisode(ctx, tc.tenant, tc.killed)
			if err != nil || found != (tc.want != "") || got.EpisodeID != tc.want || got.StaleRebindCount != tc.rebinds {
				t.Errorf("%s: %+v found=%v err=%v", tc.name, got, found, err)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
