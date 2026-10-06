package sources

import (
	"strings"
	"sync"
	"testing"
)

func TestRandomIsPrefixedAndUnique(t *testing.T) {
	t.Parallel()

	generator := Random()
	seen := map[string]bool{}
	for range 1000 {
		id := generator.New(PrefixEvent)
		if !strings.HasPrefix(id, PrefixEvent) || len(id) != len(PrefixEvent)+16 {
			t.Fatalf("id %q is not a %s-prefixed 12-byte base64url value", id, PrefixEvent)
		}
		if seen[id] {
			t.Fatalf("duplicate id %q", id)
		}
		seen[id] = true
	}
}

func TestDeterministicSequenceIsReproducible(t *testing.T) {
	t.Parallel()

	first, second := Deterministic(), Deterministic()
	for _, prefix := range []string{PrefixEpisode, PrefixAttempt, PrefixEpisode} {
		if a, b := first.New(prefix), second.New(prefix); a != b {
			t.Fatalf("deterministic generators diverged: %q != %q", a, b)
		}
	}
	if got := Deterministic().New(PrefixDecision); got != "dec_0000000000000001" {
		t.Fatalf("first deterministic id = %q", got)
	}
}

func TestGeneratorsAreSafeForConcurrentUse(t *testing.T) {
	t.Parallel()

	deterministic := Deterministic()
	var mu sync.Mutex
	seen := map[string]bool{}
	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range 100 {
				ids := []string{deterministic.New(PrefixIntent), deterministic.New(PrefixOutcome)}
				mu.Lock()
				for _, id := range ids {
					if seen[id] {
						t.Errorf("duplicate id %q under concurrency", id)
					}
					seen[id] = true
				}
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	// 8 goroutines x 100 iterations x 2 ids: the next one is call 1601 (0x641).
	if got := deterministic.New(PrefixOutcome); got != "out_0000000000000641" {
		t.Fatalf("generator did not count every concurrent call: %q", got)
	}
}
