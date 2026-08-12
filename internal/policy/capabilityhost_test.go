package policy_test

import (
	"context"
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/policy"
)

func TestCapabilityHostIsExplicitAllowlist(t *testing.T) {
	host, err := policy.NewCapabilityHost([]policy.Capability{{
		Name: "evidence_get",
		Call: func(context.Context, map[string]any) (map[string]any, error) { return map[string]any{"ok": true}, nil },
	}})
	if err != nil {
		t.Fatal(err)
	}
	result, err := host.Call(context.Background(), "evidence_get", nil)
	if err != nil || result["ok"] != true {
		t.Fatalf("allowlisted capability result=%v err=%v", result, err)
	}
	if _, err := host.Call(context.Background(), "shell", nil); err == nil {
		t.Fatal("unallowlisted capability was callable")
	}
}

func TestCapabilityHostRejectsDuplicateOrIncompleteEntries(t *testing.T) {
	if _, err := policy.NewCapabilityHost([]policy.Capability{{Name: "same", Call: func(context.Context, map[string]any) (map[string]any, error) { return nil, nil }}, {Name: "same", Call: func(context.Context, map[string]any) (map[string]any, error) { return nil, nil }}}); err == nil {
		t.Fatal("duplicate capability was accepted")
	}
	if _, err := policy.NewCapabilityHost([]policy.Capability{{Name: "missing_call"}}); err == nil {
		t.Fatal("incomplete capability was accepted")
	}
}
