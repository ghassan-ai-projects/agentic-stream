package app_test

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/contractsv1"
	"github.com/ghassan-ai-projects/agentic-stream/internal/eventlog"
	"github.com/ghassan-ai-projects/agentic-stream/internal/ingress/internal/app"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage/storagetest"
)

func feedOf(events ...contractsv1.Envelope) app.Feed {
	return func(ctx context.Context, emit func(context.Context, contractsv1.Envelope) error) error {
		for _, event := range events {
			if err := emit(ctx, event); err != nil {
				return err
			}
		}
		return nil
	}
}

func fedVibration(t *testing.T, id string, change func(*contractsv1.Envelope)) contractsv1.Envelope {
	t.Helper()
	var event contractsv1.Envelope
	if err := json.Unmarshal([]byte(vibrationLine(id, "2026-01-01T00:00:00Z")), &event); err != nil {
		t.Fatal(err)
	}
	change(&event)
	return event
}

func TestServeFeedAdmitsEachEnvelopeLikeALiveLine(t *testing.T) {
	t.Parallel()
	db := storagetest.OpenTemp(t)
	registerBuiltinSchema(t, db, "motor.vibration.observed/1.0")
	source := newApp(t, db, eventlog.NewEventLog(db).RequireSchemaValidation(), "default")
	feed := feedOf(fedVibration(t, "evt-fed-1", func(e *contractsv1.Envelope) { e.TenantID = "" }),
		fedVibration(t, "evt-fed-undeclared", func(e *contractsv1.Envelope) { e.Data = map[string]any{"unknown": 5.0} }),
		fedVibration(t, "evt-fed-unsourced", func(e *contractsv1.Envelope) { e.Source = "" }))
	var received []string
	err := source.ServeFeed(t.Context(), feed, func(_ context.Context, env contractsv1.Envelope) error {
		received = append(received, env.ID+"@"+env.TenantID)
		return nil
	})
	if err != nil || !slices.Equal(received, []string{"evt-fed-1@default"}) {
		t.Fatalf("ServeFeed = %v, sink got %v, want only evt-fed-1 for the default tenant", err, received)
	}
	want := map[string]string{"evt-fed-undeclared": "schema_invalid", "evt-fed-unsourced": "envelope_invalid"}
	if got := quarantine(t, db); len(got) != len(want) || got["evt-fed-undeclared"] != want["evt-fed-undeclared"] || got["evt-fed-unsourced"] != want["evt-fed-unsourced"] {
		t.Fatalf("quarantine = %v, want %v", got, want)
	}
}

func TestServeFeedStopsWithTheSinksError(t *testing.T) {
	t.Parallel()
	db := storagetest.OpenTemp(t)
	source := newApp(t, db, eventlog.NewEventLog(db), "default")
	failure := errors.New("pipeline down")
	sink := func(context.Context, contractsv1.Envelope) error { return failure }
	if err := source.ServeFeed(t.Context(), feedOf(fedVibration(t, "evt-fed-1", func(*contractsv1.Envelope) {})), sink); !errors.Is(err, failure) {
		t.Fatalf("ServeFeed = %v, want the sink's error", err)
	}
	if err := source.ServeFeed(t.Context(), nil, sink); !errors.Is(err, app.ErrFeedWithoutSink) {
		t.Fatalf("ServeFeed(no feed) = %v, want ErrFeedWithoutSink", err)
	}
	if err := source.ServeFeed(t.Context(), feedOf(), nil); !errors.Is(err, app.ErrFeedWithoutSink) {
		t.Fatalf("ServeFeed(no sink) = %v, want ErrFeedWithoutSink", err)
	}
}
