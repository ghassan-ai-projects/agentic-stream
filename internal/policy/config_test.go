package policy

import (
	"context"
	"database/sql"
	"strings"
	"testing"
)

func validConfig() Config {
	return Config{PolicyVersion: "v1", RuntimeOwner: func(context.Context, *sql.Tx, string) error { return nil }, DecisionEpoch: func(context.Context, *sql.Tx, string) error { return nil }}
}

func TestNewRequiresSafetyDependencies(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		change func(*Config)
		want   string
	}{
		{"owner check", func(c *Config) { c.RuntimeOwner = nil }, "ownership and decision epoch checks are required"},
		{"epoch check", func(c *Config) { c.DecisionEpoch = nil }, "ownership and decision epoch checks are required"},
		{"policy version", func(c *Config) { c.PolicyVersion = "" }, "construct policy service"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			cfg := validConfig()
			test.change(&cfg)
			if service, err := New(cfg); err == nil || service != nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("New = %v, %v; want a refusal mentioning %q", service, err, test.want)
			}
		})
	}
}

func TestNewBuildsAServiceFromACompleteConfiguration(t *testing.T) {
	t.Parallel()
	if service, err := New(validConfig()); err != nil || service == nil {
		t.Fatalf("New = %v, %v", service, err)
	}
}
