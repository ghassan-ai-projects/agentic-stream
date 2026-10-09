package store_test

import (
	"context"
	"database/sql"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/episodeledger/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/episodeledger/internal/store"
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
	within(t, func(ctx context.Context, tx *store.Tx, _ *sql.Tx) {
		want := fullAdmission(t)
		must(t, tx.InsertEpisode(ctx, want, time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)))
		got, err := tx.ReadAdmission(ctx, want.EpisodeID)
		if err != nil || !reflect.DeepEqual(got, want) {
			t.Fatalf("admission = %+v err = %v, want %+v", got, err, want)
		}
	})
}

func TestReadingAnUnknownAdmissionNamesTheEpisode(t *testing.T) {
	t.Parallel()
	within(t, func(ctx context.Context, tx *store.Tx, _ *sql.Tx) {
		_, err := tx.ReadAdmission(ctx, "missing")
		if !errors.Is(err, sql.ErrNoRows) || err.Error() != "load episode admission missing: sql: no rows in result set" {
			t.Fatalf("ReadAdmission(missing) = %v, want ErrNoRows naming the episode", err)
		}
	})
}

func TestAnAdmittedEpisodeStartsAdmittedAtTheNanosecondItWasAccepted(t *testing.T) {
	t.Parallel()
	within(t, func(ctx context.Context, tx *store.Tx, raw *sql.Tx) {
		must(t, tx.InsertEpisode(ctx, admitted("e1"), time.Date(2026, 10, 2, 12, 0, 0, 123456789, time.UTC)))
		state := queryText(t, ctx, raw, "SELECT lifecycle_status || '|' || accepted_at || '|' || current_fence FROM episodes WHERE episode_id = 'e1'")
		if want := "admitted|2026-10-02T12:00:00.123456789Z|0"; state != want {
			t.Fatalf("episode row = %s, want %s", state, want)
		}
	})
}

func TestAnEpisodeWithoutADeclaredDispatchPolicyIsRefused(t *testing.T) {
	t.Parallel()
	within(t, func(ctx context.Context, tx *store.Tx, _ *sql.Tx) {
		undeclared := admitted("e1")
		undeclared.DispatchPolicy = ""
		err := tx.InsertEpisode(ctx, undeclared, at)
		if err == nil || !strings.Contains(err.Error(), "insert episode") || !strings.Contains(err.Error(), "dispatch_policy") {
			t.Fatalf("InsertEpisode without dispatch policy = %v, want the dispatch_policy CHECK", err)
		}
	})
}

func TestEpisodeIdentityAndTheLiveEpisodeRuleAreEnforcedByTheDatabase(t *testing.T) {
	t.Parallel()
	within(t, func(ctx context.Context, tx *store.Tx, raw *sql.Tx) {
		admit(t, ctx, tx, "e1")
		sameId := admitted("e1")
		sameId.SituationID = "elsewhere"
		duplicate := tx.InsertEpisode(ctx, sameId, at)
		if duplicate == nil || store.IsLiveEpisodeViolation(duplicate) {
			t.Fatalf("duplicate episode id = %v, want a refusal that is not the live-episode violation", duplicate)
		}
		sameSituation := admitted("e2")
		sameSituation.SituationID = "s-e1"
		if err := tx.InsertEpisode(ctx, sameSituation, at); !store.IsLiveEpisodeViolation(err) {
			t.Fatalf("second live episode for a situation = %v, want the live-episode violation", err)
		}
		execSQL(t, ctx, raw, "UPDATE episodes SET lifecycle_status = 'concluded' WHERE episode_id = 'e1'")
		must(t, tx.InsertEpisode(ctx, sameSituation, at))
	})
}
