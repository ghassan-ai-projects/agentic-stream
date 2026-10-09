package replay_test

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/replay"
)

func TestRunRecordedVerifiesAgainstTheSourceDatabase(t *testing.T) {
	t.Parallel()
	source := newRequest(t)
	if _, err := replay.Run(seededContext(t), source); err != nil {
		t.Fatalf("replay the source runtime: %v", err)
	}
	result, err := replay.RunRecorded(seededContext(t), newRequest(t), source.DBPath)
	if err != nil {
		t.Fatal(err)
	}
	if result.Mode != replay.ModeRecorded || result.WorkerInvoked || result.EffectsAllowed {
		t.Fatalf("recorded result = %+v", result)
	}
}

func TestRunRecordedNamesAMissingSourceDatabase(t *testing.T) {
	t.Parallel()
	_, err := replay.RunRecorded(seededContext(t), newRequest(t), filepath.Join(t.TempDir(), "missing.db"))
	if err == nil || !strings.Contains(err.Error(), "source database") {
		t.Fatalf("RunRecorded over a missing source = %v, want a source database error", err)
	}
}

func TestRunShadowRefusesARelativeWorkerSocket(t *testing.T) {
	t.Parallel()
	_, err := replay.RunShadow(seededContext(t), newRequest(t), "worker.sock", "tamoz")
	if err == nil || !strings.Contains(err.Error(), "connect shadow candidate worker.sock") {
		t.Fatalf("RunShadow with a relative socket = %v, want connect shadow candidate failure", err)
	}
}
