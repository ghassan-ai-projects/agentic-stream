package native_test

import (
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/executor/native"
)

func TestFacadeConstructsExecutorAndStores(t *testing.T) {
	t.Parallel()
	if _, err := native.New(native.Config{Provider: &native.DeterministicProvider{}}); err != nil {
		t.Fatalf("new executor: %v", err)
	}
	if native.NewMemoryArtifactStore() == nil {
		t.Fatal("memory store is nil")
	}
	if native.NewSQLiteEvidenceTool(nil, "evidence", "tenant", "entity").Name() != "evidence" {
		t.Fatal("evidence tool name lost")
	}
	if native.ErrInterrupt == nil {
		t.Fatal("interrupt sentinel missing")
	}
}
