package domain

import (
	"strings"
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/spec"
)

func TestBindExecutorAddsASectionAndRenamesTheExecutor(t *testing.T) {
	t.Parallel()
	req := &Request{EpisodeID: "ep-1", ExecutorName: "tamoz", RequestJSON: []byte(`{"snapshot":{"facts":{"facts.level":4},"evidence":["e1"]}}`)}
	facts, evidence, err := req.SnapshotFacts()
	if err != nil || facts["facts.level"] != 4.0 || len(evidence) != 1 {
		t.Fatalf("snapshot facts = %v %v %v", facts, evidence, err)
	}
	if err := req.BindExecutor("rule", "sha256:pb", "rule", map[string]any{"rule_id": "r"}); err != nil {
		t.Fatal(err)
	}
	if req.ExecutorName != "rule" || req.ExecutorVersion != "sha256:pb" || !strings.Contains(string(req.RequestJSON), `"rule":{"rule_id":"r"}`) {
		t.Fatalf("bound request = %s %s %s", req.ExecutorName, req.ExecutorVersion, req.RequestJSON)
	}
	if err := req.BindExecutor("rule", "sha256:pb", "snapshot", nil); err == nil || !strings.Contains(err.Error(), "already has a snapshot section") {
		t.Fatalf("binding over the snapshot = %v, want it refused", err)
	}
	req.DispatchInShadow()
	if req.DispatchPolicy != spec.DispatchShadow {
		t.Fatalf("dispatch policy = %q, want shadow", req.DispatchPolicy)
	}
}
