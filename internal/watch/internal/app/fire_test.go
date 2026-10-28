package app_test

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/sources"
)

func TestAWatchFiresOncePerEventAndOnlyWithinItsAllowance(t *testing.T) {
	t.Parallel()
	db := openDB(t)
	service := newService(t, db)
	install(t, service, watchCommand("cmd-1", "features.temperature > 90", "motor-1", 1, fixtureNow.Add(time.Hour)))
	hot := map[string]any{"temperature": 95}

	if fired, err := service.FireEvent(t.Context(), "evt-1", "motor-1", featuresOf(hot)); err != nil || fired != 1 {
		t.Fatalf("first fire = %d, %v; want 1", fired, err)
	}
	if fired, err := service.FireEvent(t.Context(), "evt-1", "motor-1", featuresOf(hot)); err != nil || fired != 0 {
		t.Fatalf("redelivered event fired %d, %v; want the duplicate absorbed", fired, err)
	}
	if fired, err := service.FireEvent(t.Context(), "evt-2", "motor-1", featuresOf(hot)); err != nil || fired != 0 {
		t.Fatalf("event after the allowance fired %d, %v; want none", fired, err)
	}
	if got, want := readWatch(t, db, "cmd-1"), (watchState{Status: "disabled", Remaining: 0, Fires: 1}); got != want {
		t.Fatalf("watch = %+v, want %+v", got, want)
	}
	install(t, service, watchCommand("cmd-1", "features.temperature > 90", "motor-1", 1, fixtureNow.Add(time.Hour)))
	if got := readWatch(t, db, "cmd-1"); got.Status != "disabled" {
		t.Fatalf("reinstalling a spent watch revived it: %+v", got)
	}
}

func TestAWatchFiresOnlyWhenItsExpressionHoldsForTheEventFeatures(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name       string
		expression string
		features   map[string]any
		want       int
	}{
		{"threshold crossed", "features.temperature > 90", map[string]any{"temperature": 95}, 1},
		{"threshold not crossed", "features.temperature > 90", map[string]any{"temperature": 10}, 0},
		{"fallback condition score reached", "features.condition_score >= 0.8", map[string]any{"condition_score": 0.9}, 1},
		{"fallback condition score below", "features.condition_score >= 0.8", map[string]any{"condition_score": 0.5}, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			db := openDB(t)
			service := newService(t, db)
			install(t, service, watchCommand("cmd-1", tc.expression, "zone-1", 1, fixtureNow.Add(time.Hour)))
			if fired, err := service.FireEvent(t.Context(), "evt-1", "zone-1", featuresOf(tc.features)); err != nil || fired != tc.want {
				t.Fatalf("fired = %d, %v; want %d", fired, err, tc.want)
			}
		})
	}
}

func TestAnExpressionThatCannotBeEvaluatedIsASkippedNoFireUntilItsDataArrives(t *testing.T) {
	t.Parallel()
	db := openDB(t)
	service := newService(t, db)
	install(t, service, watchCommand("cmd-1", "features.do_mean_15m > 0", "motor-1", 1, fixtureNow.Add(time.Hour)))

	if fired, err := service.Fire(t.Context(), "cmd-1", "evt-missing-key", "sit-1", "motor-1", map[string]any{"temperature": 95}); err != nil || fired {
		t.Fatalf("missing key: fired = %v, err = %v; want a skipped evaluation, not an error", fired, err)
	}
	if got, want := readWatch(t, db, "cmd-1"), (watchState{Status: "active", Remaining: 1}); got != want {
		t.Fatalf("watch after the skipped evaluation = %+v, want %+v", got, want)
	}
	if fired, err := service.Fire(t.Context(), "cmd-1", "evt-present-key", "sit-1", "motor-1", map[string]any{"do_mean_15m": 15}); err != nil || !fired {
		t.Fatalf("present key: fired = %v, err = %v; want a fire", fired, err)
	}
	if got := readWatch(t, db, "cmd-1"); got.Fires != 1 {
		t.Fatalf("watch fires = %d, want 1", got.Fires)
	}
}

func TestAWatchIgnoresEventsOfAnotherSituationOrTarget(t *testing.T) {
	t.Parallel()
	db := openDB(t)
	service := newService(t, db)
	install(t, service, installable("cmd-1"))
	hot := map[string]any{"temperature": 95}
	if fired, err := service.FireEvent(t.Context(), "evt-1", "motor-2", featuresOf(hot)); err != nil || fired != 0 {
		t.Fatalf("event of another target fired %d, %v", fired, err)
	}
	if fired, err := service.Fire(t.Context(), "cmd-1", "evt-2", "sit-2", "motor-1", hot); err != nil || fired {
		t.Fatalf("event of another Situation fired = %v, %v", fired, err)
	}
	if fired, err := service.Fire(t.Context(), "cmd-1", "evt-3", "sit-1", "motor-2", hot); err != nil || fired {
		t.Fatalf("event for another target through Fire fired = %v, %v", fired, err)
	}
	if got := readWatch(t, db, "cmd-1"); got.Fires != 0 || got.Remaining != 2 {
		t.Fatalf("watch = %+v, want it untouched", got)
	}
}

func TestOneEventFiresEveryMatchingWatchOfItsTarget(t *testing.T) {
	t.Parallel()
	db := openDB(t)
	service := newService(t, db)
	install(t, service, installable("cmd-1"))
	install(t, service, watchCommand("cmd-2", "features.temperature > 50", "motor-1", 1, fixtureNow.Add(time.Hour)))
	install(t, service, watchCommand("cmd-3", "features.temperature > 99", "motor-1", 1, fixtureNow.Add(time.Hour)))
	if fired, err := service.FireEvent(t.Context(), "evt-1", "motor-1", featuresOf(map[string]any{"temperature": 95})); err != nil || fired != 2 {
		t.Fatalf("fired = %d, %v; want the two watches whose expressions hold", fired, err)
	}
}

func TestAFireNeedsAnEventAndAWatchIdentity(t *testing.T) {
	t.Parallel()
	service := newService(t, openDB(t))
	cases := map[string]func() error{
		"event without an ID":     func() error { _, err := service.FireEvent(t.Context(), "", "motor-1", featuresOf(nil)); return err },
		"event without a target":  func() error { _, err := service.FireEvent(t.Context(), "evt-1", "", featuresOf(nil)); return err },
		"fire without a watch ID": func() error { _, err := service.Fire(t.Context(), "", "evt-1", "sit-1", "motor-1", nil); return err },
		"fire without an event":   func() error { _, err := service.Fire(t.Context(), "cmd-1", "", "sit-1", "motor-1", nil); return err },
	}
	for name, fire := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if err := fire(); err == nil {
				t.Fatal("fire was accepted")
			}
		})
	}
}

func TestAWatchFiresUpToItsExpiryAndNotAtIt(t *testing.T) {
	t.Parallel()
	db := openDB(t)
	clock := sources.NewVirtual(fixtureNow)
	service := newServiceAt(t, db, clock)
	expiresAt := fixtureNow.Add(time.Minute)
	install(t, service, watchCommand("cmd-1", "features.temperature > 90", "motor-1", 2, expiresAt))
	hot := map[string]any{"temperature": 95}

	clock.Advance(time.Minute - time.Nanosecond)
	if fired, err := service.FireEvent(t.Context(), "evt-1", "motor-1", featuresOf(hot)); err != nil || fired != 1 {
		t.Fatalf("one nanosecond before expiry fired %d, %v; want 1", fired, err)
	}
	clock.Advance(time.Nanosecond)
	if fired, err := service.FireEvent(t.Context(), "evt-2", "motor-1", featuresOf(hot)); err != nil || fired != 0 {
		t.Fatalf("at expiry fired %d, %v; want none", fired, err)
	}
}

func TestAWatchReadsTheFeaturesOfItsOwnSituation(t *testing.T) {
	t.Parallel()
	db := openDB(t)
	service := newService(t, db)
	install(t, service, installable("cmd-1"))
	var asked []string
	source := func(_ context.Context, situationID string) (map[string]any, bool, error) {
		asked = append(asked, situationID)
		return map[string]any{"temperature": 95}, true, nil
	}
	if fired, err := service.FireEvent(t.Context(), "evt-1", "motor-1", source); err != nil || fired != 1 {
		t.Fatalf("fired = %d, %v; want 1", fired, err)
	}
	if !slices.Equal(asked, []string{"sit-1"}) {
		t.Fatalf("features asked for situations %v, want only the watch's sit-1", asked)
	}
}

func TestASituationWithoutFeaturesIsASkippedNoFireAndASourceFailureIsAnError(t *testing.T) {
	t.Parallel()
	db := openDB(t)
	service := newService(t, db)
	install(t, service, installable("cmd-1"))
	unknown := func(context.Context, string) (map[string]any, bool, error) { return nil, false, nil }
	if fired, err := service.FireEvent(t.Context(), "evt-1", "motor-1", unknown); err != nil || fired != 0 {
		t.Fatalf("unknown situation: fired = %d, %v; want a skipped no-fire", fired, err)
	}
	failure := errors.New("database is gone")
	broken := func(context.Context, string) (map[string]any, bool, error) { return nil, false, failure }
	if _, err := service.FireEvent(t.Context(), "evt-2", "motor-1", broken); !errors.Is(err, failure) || !strings.Contains(err.Error(), "sit-1") {
		t.Fatalf("failing source = %v, want its error naming the situation", err)
	}
	if got := readWatch(t, db, "cmd-1"); got.Fires != 0 || got.Remaining != 2 {
		t.Fatalf("watch = %+v, want it untouched", got)
	}
}
