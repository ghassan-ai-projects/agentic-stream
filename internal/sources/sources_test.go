package sources_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/sources"
)

func TestFacadeExposesPhysicalAndVirtualClocks(t *testing.T) {
	t.Parallel()
	if got := sources.Quality(sources.Physical()); got != "physical" {
		t.Fatalf("Quality(Physical()) = %q, want physical", got)
	}
	start := time.Unix(10, 0)
	virtual := sources.NewVirtual(start)
	if got := sources.Quality(virtual); got != "virtual" {
		t.Fatalf("Quality(NewVirtual()) = %q, want virtual", got)
	}
	if !virtual.Now().Equal(start) {
		t.Fatalf("virtual clock starts at %v, want %v", virtual.Now(), start)
	}
}

func TestFacadeGeneratesRandomAndDeterministicIdentifiers(t *testing.T) {
	t.Parallel()
	random := sources.Random()
	if first, second := random.New(sources.PrefixEvent), random.New(sources.PrefixEvent); !strings.HasPrefix(first, sources.PrefixEvent) || first == second {
		t.Fatalf("random ids %q and %q must be prefixed and distinct", first, second)
	}
	first, second := sources.Deterministic(), sources.Deterministic()
	if a, b := first.New(sources.PrefixEpisode), second.New(sources.PrefixEpisode); a != b {
		t.Fatalf("deterministic generators must replay the same sequence: %q != %q", a, b)
	}
}

func TestOrPhysicalAndOrRandomKeepTheCallersChoice(t *testing.T) {
	t.Parallel()
	virtual := sources.NewVirtual(time.Date(2026, 8, 12, 12, 0, 0, 0, time.UTC))
	if got := sources.OrPhysical(virtual); got != sources.Clock(virtual) {
		t.Fatalf("OrPhysical replaced the supplied clock with %T", got)
	}
	if got := sources.Quality(sources.OrPhysical(nil)); got != "physical" {
		t.Fatalf("OrPhysical(nil) quality = %q, want physical", got)
	}
	deterministic := sources.Deterministic()
	if sources.OrRandom(deterministic) != deterministic {
		t.Fatal("OrRandom replaced the supplied generator")
	}
	if first, second := sources.OrRandom(nil).New("id_"), sources.OrRandom(nil).New("id_"); first == second {
		t.Fatalf("the default generator repeated %q", first)
	}
}

func TestNowUTCReadsTheConfiguredClockInUTCOrThePhysicalClock(t *testing.T) {
	t.Parallel()
	local := time.Date(2026, 8, 12, 15, 0, 0, 0, time.FixedZone("plus3", 3*60*60))
	configured := func() time.Time { return local }
	if got := sources.NowUTC(configured); !got.Equal(local) || got.Location() != time.UTC {
		t.Fatalf("NowUTC(configured) = %v, want %v in UTC", got, local)
	}
	if got := sources.NowFunc(configured)(); !got.Equal(local) || got.Location() != time.UTC {
		t.Fatalf("NowFunc(configured)() = %v, want %v in UTC", got, local)
	}
	before := time.Now()
	for name, got := range map[string]time.Time{"NowUTC": sources.NowUTC(nil), "NowFunc": sources.NowFunc(nil)()} {
		if got.Before(before) || time.Since(got) > time.Minute || got.Location() != time.UTC {
			t.Errorf("%s(nil) = %v, want the current time in UTC", name, got)
		}
	}
}

func TestOrLeaseDefaultsAnUnspecifiedLease(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		in, want time.Duration
	}{
		{"zero", 0, sources.DefaultLease},
		{"negative", -time.Second, sources.DefaultLease},
		{"smallest positive", time.Nanosecond, time.Nanosecond},
		{"chosen", 2 * time.Minute, 2 * time.Minute},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := sources.OrLease(tt.in); got != tt.want {
				t.Fatalf("OrLease(%v) = %v, want %v", tt.in, got, tt.want)
			}
		})
	}
}

func TestFacadeDetachedContextDropsCancellationAndIsBounded(t *testing.T) {
	t.Parallel()
	parent, cancelParent := context.WithCancel(t.Context())
	cancelParent()
	detached, cancel := sources.DetachedContext(parent)
	defer cancel()
	if detached.Err() != nil {
		t.Fatalf("detached context inherited cancellation: %v", detached.Err())
	}
	deadline, ok := detached.Deadline()
	if remaining := time.Until(deadline); !ok || remaining > sources.PersistGrace || remaining <= 0 {
		t.Fatalf("detached context must expire within PersistGrace: deadline=%v ok=%v remaining=%v", deadline, ok, remaining)
	}
}
