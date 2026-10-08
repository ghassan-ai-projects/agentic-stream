package sources_test

import (
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
