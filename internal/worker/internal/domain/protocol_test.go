package domain

import (
	"testing"
	"time"

	"google.golang.org/protobuf/types/known/durationpb"

	runtimev1 "github.com/ghassan-ai-projects/agentic-stream/proto/agenticstream/runtime/v1"
)

func TestSocketPathRuleAcceptsOnlyCleanAbsoluteUnixPaths(t *testing.T) {
	t.Parallel()
	for _, bad := range []string{"", "relative.sock", "unix:///tmp/x.sock", "/tmp/../x.sock", "/tmp/x\x00.sock"} {
		if ValidateEvidenceSocketPath(bad) == nil {
			t.Errorf("%q accepted", bad)
		}
	}
	if err := ValidateEvidenceSocketPath("/tmp/evidence.sock"); err != nil {
		t.Fatal(err)
	}
}

func TestBudgetRequiresAPositiveWallTime(t *testing.T) {
	t.Parallel()
	if ValidateBudget(nil) == nil || ValidateBudget(&runtimev1.EpisodeBudget{}) == nil {
		t.Fatal("budget without wall time accepted")
	}
	if ValidateBudget(&runtimev1.EpisodeBudget{WallTime: durationpb.New(0)}) == nil {
		t.Fatal("zero wall time accepted")
	}
	if err := ValidateBudget(&runtimev1.EpisodeBudget{WallTime: durationpb.New(time.Second)}); err != nil {
		t.Fatal(err)
	}
}
