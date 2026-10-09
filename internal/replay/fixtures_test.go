package replay_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/replay"
)

const (
	predictiveSpec  = "../../docs/design/examples/predictive-maintenance.situation.yaml"
	predictiveTrace = "../../examples/predictive-maintenance/testdata/trace-opening.jsonl"
)

func seededContext(t *testing.T) context.Context {
	t.Helper()
	return replay.WithSeededDatabases(t.Context())
}

func newRequest(t *testing.T) replay.Request {
	t.Helper()
	return replay.Request{DBPath: filepath.Join(t.TempDir(), "replay.db"), SpecPath: predictiveSpec, TracePath: predictiveTrace, TenantID: "default"}
}
