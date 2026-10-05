package policy

import (
	"context"
	"database/sql"
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
