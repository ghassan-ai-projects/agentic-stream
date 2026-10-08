package sources_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/sources"
)

func TestFacadeExposesPhysicalAndVirtualTime(t *testing.T) {
	t.Parallel()
	if sources.Quality(sources.Physical()) != "physical" {
		t.Fatal("physical clock must report physical quality")
	}
	virtual := sources.NewVirtual(time.Unix(10, 0))
	if sources.Quality(virtual) != "virtual" || !virtual.Now().Equal(time.Unix(10, 0)) {
		t.Fatal("virtual clock must start where it was told")
	}
	virtual.Advance(time.Second)
	if !virtual.Now().Equal(time.Unix(11, 0)) {
		t.Fatal("virtual clock did not advance")
	}
}

func TestFacadeGeneratesRandomAndDeterministicIdentifiers(t *testing.T) {
	t.Parallel()
	if id := sources.Random().New(sources.PrefixEvent); !strings.HasPrefix(id, sources.PrefixEvent) || id == sources.Random().New(sources.PrefixEvent) {
		t.Fatalf("random id %q is not unique and prefixed", id)
	}
	first, second := sources.Deterministic(), sources.Deterministic()
	if first.New(sources.PrefixEpisode) != second.New(sources.PrefixEpisode) {
		t.Fatal("deterministic generators must replay the same sequence")
	}
}

func TestOrPhysicalAndOrRandomKeepTheCallersChoice(t *testing.T) {
	t.Parallel()
	virtual := sources.NewVirtual(time.Date(2026, 8, 12, 12, 0, 0, 0, time.UTC))
	if sources.OrPhysical(virtual) != sources.Clock(virtual) || sources.Quality(sources.OrPhysical(nil)) != "physical" {
		t.Fatal("OrPhysical must keep a supplied clock and default a nil one to the physical clock")
	}
	deterministic := sources.Deterministic()
	if sources.OrRandom(deterministic) != deterministic {
		t.Fatal("OrRandom replaced a supplied generator")
	}
	if first, second := sources.OrRandom(nil).New("id_"), sources.OrRandom(nil).New("id_"); first == second {
		t.Fatalf("the default generator repeated %q", first)
	}
}

func TestOrLeaseDefaultsAnUnspecifiedLease(t *testing.T) {
	t.Parallel()
	for name, tc := range map[string]struct{ in, want time.Duration }{
		"zero":     {0, sources.DefaultLease},
		"negative": {-time.Second, sources.DefaultLease},
		"chosen":   {2 * time.Minute, 2 * time.Minute},
	} {
		if got := sources.OrLease(tc.in); got != tc.want {
			t.Errorf("%s: OrLease(%v) = %v, want %v", name, tc.in, got, tc.want)
		}
	}
}

func TestFormatTimeIsUTCWithNanoseconds(t *testing.T) {
	t.Parallel()
	local := time.Date(2026, 8, 12, 14, 0, 0, 123456789, time.FixedZone("plus2", 2*3600))
	if got, want := sources.FormatTime(local), "2026-08-12T12:00:00.123456789Z"; got != want {
		t.Fatalf("FormatTime = %q, want %q", got, want)
	}
}

type detachedKey struct{}

func TestDetachedContextKeepsValuesDropsCancellationAndIsBounded(t *testing.T) {
	t.Parallel()
	parent, cancelParent := context.WithCancel(context.WithValue(t.Context(), detachedKey{}, "trace"))
	cancelParent()
	detached, cancel := sources.DetachedContext(parent)
	defer cancel()
	if detached.Err() != nil {
		t.Fatalf("detached context inherited cancellation: %v", detached.Err())
	}
	if detached.Value(detachedKey{}) != "trace" {
		t.Fatal("detached context lost the caller's values")
	}
	deadline, ok := detached.Deadline()
	if !ok || time.Until(deadline) > sources.PersistGrace || time.Until(deadline) <= 0 {
		t.Fatalf("detached context is not bounded by PersistGrace: deadline=%v ok=%v", deadline, ok)
	}
	if sources.PersistGrace != 5*time.Second {
		t.Fatalf("PersistGrace = %v, want 5s", sources.PersistGrace)
	}
}
