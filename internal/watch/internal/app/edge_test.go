package app_test

import (
	"context"
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/sources"
	"github.com/ghassan-ai-projects/agentic-stream/internal/watch/internal/app"
	"github.com/ghassan-ai-projects/agentic-stream/internal/watch/internal/store"
)

func TestAWatchFiresWhenItsExpressionTurnsTrueAndNotAgainWhileItHolds(t *testing.T) {
	t.Parallel()
	db := openDB(t)
	service := newService(t, db)
	install(t, service, watchCommand("cmd-1", "features.temperature > 90", "motor-1", 3, fixtureNow.Add(time.Hour)))
	for _, step := range []struct {
		event       string
		temperature float64
		fires       int
	}{
		{"evt-1", 95, 1}, {"evt-2", 96, 0}, {"evt-3", 97, 0}, {"evt-4", 80, 0}, {"evt-5", 95, 1},
	} {
		fired, err := service.FireEvent(t.Context(), step.event, "motor-1", featuresOf(map[string]any{"temperature": step.temperature}))
		if err != nil || len(fired) != step.fires {
			t.Fatalf("%s at %v fired %d, %v; want %d", step.event, step.temperature, len(fired), err, step.fires)
		}
	}
	if got, want := readWatch(t, db, "cmd-1"), (watchState{Status: "active", Remaining: 1, Fires: 2}); got != want {
		t.Fatalf("watch = %+v, want %+v: one fire per rise", got, want)
	}
}

func TestAWatchWhoseExpressionHoldsAtInstallFiresOnlyAfterItHasBeenFalse(t *testing.T) {
	t.Parallel()
	db := openDB(t)
	temperature := 95.0
	source := func(context.Context, string) (map[string]any, bool, error) {
		return map[string]any{"temperature": temperature}, true, nil
	}
	service, err := app.New(app.Config{Store: store.New(db, allowOwner, "epoch"), Clock: sources.NewVirtual(fixtureNow), Features: source})
	if err != nil {
		t.Fatal(err)
	}
	install(t, service, watchCommand("cmd-1", "features.temperature > 90", "motor-1", 3, fixtureNow.Add(time.Hour)))
	if fired, err := service.FireEvent(t.Context(), "evt-1", "motor-1", source); err != nil || len(fired) != 0 {
		t.Fatalf("an expression true since install fired %d, %v; want none", len(fired), err)
	}
	temperature = 80
	if fired, err := service.FireEvent(t.Context(), "evt-2", "motor-1", source); err != nil || len(fired) != 0 {
		t.Fatalf("a false expression fired %d, %v", len(fired), err)
	}
	temperature = 95
	if fired, err := service.FireEvent(t.Context(), "evt-3", "motor-1", source); err != nil || len(fired) != 1 {
		t.Fatalf("the expression turning true again fired %d, %v; want 1", len(fired), err)
	}
}
