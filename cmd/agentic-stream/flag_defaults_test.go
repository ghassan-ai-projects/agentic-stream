package main

import (
	"testing"

	"github.com/spf13/cobra"

	"github.com/ghassan-ai-projects/agentic-stream/internal/contractsv1"
	"github.com/ghassan-ai-projects/agentic-stream/internal/sources"
)

func TestFlagDefaultsComeFromTheirOwners(t *testing.T) {
	t.Parallel()
	want := map[string]string{"tenant": contractsv1.TenantID, "owner-lease": sources.DefaultLease.String()}
	seen := map[string]int{}
	var walk func(*cobra.Command)
	walk = func(command *cobra.Command) {
		for name, def := range want {
			if flag := command.Flags().Lookup(name); flag != nil && flag.DefValue != "" {
				seen[name]++
				if flag.DefValue != def {
					t.Errorf("%s --%s default = %q, want %q", command.Name(), name, flag.DefValue, def)
				}
			}
		}
		for _, child := range command.Commands() {
			walk(child)
		}
	}
	walk(newRootCommand())
	for name := range want {
		if seen[name] == 0 {
			t.Errorf("no command defines --%s", name)
		}
	}
}
