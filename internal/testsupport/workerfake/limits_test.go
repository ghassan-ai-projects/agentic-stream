package workerfake

import (
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/worker"
)

func TestLimitsResolveEveryZeroBoundToItsDefault(t *testing.T) {
	t.Parallel()
	got := Limits{MaxEvents: 7}.Resolved()
	want := Limits{MaxRequestBytes: defaultMaxRequestBytes, MaxEventBytes: defaultMaxEventBytes, MaxEvents: 7, MaxStreamBytes: worker.DefaultMaxStreamBytes}
	if got != want {
		t.Fatalf("Resolved() = %+v, want %+v", got, want)
	}
	if again := got.Resolved(); again != got {
		t.Fatalf("resolving twice changed the limits: %+v", again)
	}
}
