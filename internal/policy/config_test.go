package policy

import (
	"context"
	"database/sql"
	"errors"
	"github.com/ghassan-ai-projects/agentic-stream/internal/interlock"
	"testing"
)

func validConfig() Config {
	return Config{PolicyVersion: "v1", RuntimeOwner: func(context.Context, *sql.Tx, string) error { return nil }, DecisionEpoch: func(context.Context, *sql.Tx, string) error { return nil }, Interlock: interlock.DurableReader{}}
}
func TestNewRequiresSafetyDependencies(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name   string
		change func(*Config)
	}{
		{"owner", func(c *Config) { c.RuntimeOwner = nil }}, {"epoch", func(c *Config) { c.DecisionEpoch = nil }}, {"interlock", func(c *Config) { c.Interlock = nil }}, {"version", func(c *Config) { c.PolicyVersion = "" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := validConfig()
			tc.change(&cfg)
			if s, err := New(cfg); err == nil || s != nil {
				t.Fatal("invalid configuration accepted")
			}
		})
	}
}
func TestFacadeDelegatesOwnershipFailure(t *testing.T) {
	t.Parallel()
	sentinel := errors.New("lease lost")
	cfg := validConfig()
	cfg.OwnerEpoch = "owner"
	calls := 0
	cfg.RuntimeOwner = func(_ context.Context, tx *sql.Tx, epoch string) error {
		calls++
		if tx != nil || epoch != "owner" {
			t.Fatal("ownership inputs changed")
		}
		return sentinel
	}
	service, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	r, err := service.EvaluateIntent(t.Context(), nil, EvaluationRequest{IntentID: "intent"})
	if !errors.Is(err, sentinel) || r.IntentID != "intent" {
		t.Fatal(r, err)
	}
	r, err = service.ResolveApproval(t.Context(), nil, ApprovalResolution{ID: "approval"})
	if !errors.Is(err, sentinel) || r.ApprovalID != "approval" || calls != 2 {
		t.Fatal(r, err, calls)
	}
}
func TestDefinitionFacade(t *testing.T) {
	t.Parallel()
	digest, err := DigestForVersion("v1")
	if err != nil || digest == "" {
		t.Fatal(digest, err)
	}
	if CanonicalDocumentForVersion("v1")["policy_version"] != "v1" {
		t.Fatal("definition delegation")
	}
	signed, err := ApprovalAssertionSigningBytes(ApprovalAssertion{ApprovalID: "approval"})
	if err != nil || len(signed) == 0 {
		t.Fatal(err)
	}
}
