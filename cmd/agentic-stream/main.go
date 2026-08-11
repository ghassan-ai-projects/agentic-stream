// Package main is the entrypoint for the agentic-stream CLI and local runtime.
package main

import (
	"context"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/ghassan-ai-projects/agentic-stream/internal/spec"
)

// Version metadata injected at build time.
var (
	Version = "dev"
	Commit  = "none"
)

func main() {
	if err := newRootCommand().Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func newRootCommand() *cobra.Command {
	root := &cobra.Command{
		Use:   "agentic-stream",
		Short: "Streaming-native agent runtime (Situation Runtime).",
		Long: `Agentic Stream continuously converts unbounded evidence into durable,
versioned Situations and starts bounded agent episodes only when a deterministic
cognitive scheduler decides reasoning is useful.`,
		SilenceUsage:  true,
		SilenceErrors: true,
	}

	root.AddCommand(newVersionCommand())
	root.AddCommand(newValidateCommand())
	root.AddCommand(newConfigEffectiveCommand())

	return root
}

func newVersionCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print version and build metadata.",
		Run: func(cmd *cobra.Command, _ []string) {
			cmd.Printf("agentic-stream version %s (commit %s)\n", Version, Commit)
		},
	}
}

func newValidateCommand() *cobra.Command {
	var outputJSON bool

	cmd := &cobra.Command{
		Use:   "validate <spec.yaml>",
		Short: "Validate and compile a SituationSpec.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			path := args[0]
			result, err := spec.CompileFile(context.Background(), path)
			if err != nil {
				return fmt.Errorf("compile %s: %w", path, err)
			}

			if outputJSON {
				cmd.Printf("%s\n", result.CanonicalJSON)
				return nil
			}

			cmd.Printf("ok: %s\n", result.Metadata.Name)
			cmd.Printf("version: %s\n", result.Metadata.Version)
			cmd.Printf("digest: %s\n", result.Digest)
			cmd.Printf("schema: %s\n", result.SchemaVersion)
			return nil
		},
	}

	cmd.Flags().BoolVar(&outputJSON, "json", false, "Emit canonical JSON instead of summary")
	return cmd
}

func newConfigEffectiveCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "config effective",
		Short: "Show effective runtime configuration (placeholder).",
		Run: func(_ *cobra.Command, _ []string) {
			fmt.Println("config effective: not yet implemented")
		},
	}
}
