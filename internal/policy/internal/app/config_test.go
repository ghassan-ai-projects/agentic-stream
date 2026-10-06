package app_test

import (
	"context"
	"database/sql"
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/interlock"
	"github.com/ghassan-ai-projects/agentic-stream/internal/policy"
	"github.com/ghassan-ai-projects/agentic-stream/internal/sources"
)

func newTestService(t *testing.T, configure ...func(*policy.Config)) *policy.Service {
	t.Helper()
	cfg := policy.Config{PolicyVersion: "policy-v1", IDGenerator: sources.Deterministic(), RuntimeOwner: unownedCheck, DecisionEpoch: unownedCheck, Interlock: interlock.DurableReader{}}
	for _, change := range configure {
		change(&cfg)
	}
	service, err := policy.New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	return service
}
func unownedCheck(context.Context, *sql.Tx, string) error { return nil }
