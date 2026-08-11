// Package main is the entrypoint for the agentic-stream CLI and local runtime.
package main

import (
	"context"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/ghassan-ai-projects/agentic-stream/internal/contractsv1"
	"github.com/ghassan-ai-projects/agentic-stream/internal/replay"
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
	root.AddCommand(newRunCommand())
	root.AddCommand(newConfigEffectiveCommand())

	return root
}

func newVersionCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print version and build metadata.",
		Run: func(cmd *cobra.Command, _ []string) {
			cmd.Printf("agentic-stream version %s (commit %s)\n", Version, Commit)
			cmd.Printf("contract: %s\n", contractsv1.ContractVersion)
			cmd.Printf("protocol: %s\n", contractsv1.ProtocolVersion)
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

func newRunCommand() *cobra.Command {
	var (
		dbPath   string
		tenantID string
	)

	cmd := &cobra.Command{
		Use:   "run --spec <spec.yaml> --trace <trace.jsonl>",
		Short: "Replay a JSONL trace against a spec and print the canonical result.",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			specPath, err := cmd.Flags().GetString("spec")
			if err != nil {
				return fmt.Errorf("get spec flag: %w", err)
			}
			tracePath, err := cmd.Flags().GetString("trace")
			if err != nil {
				return fmt.Errorf("get trace flag: %w", err)
			}
			if specPath == "" || tracePath == "" {
				return fmt.Errorf("--spec and --trace are required")
			}
			if dbPath == "" {
				dbPath = tracePath + ".replay.db"
			}

			result, err := replay.Run(cmd.Context(), dbPath, specPath, tracePath, tenantID)
			if err != nil {
				return fmt.Errorf("run replay: %w", err)
			}

			cmd.Printf("events_processed=%d situation_versions=%d versions_hash=%s\n",
				result.EventsProcessed, result.VersionCount, result.VersionsHash)
			return nil
		},
	}

	cmd.Flags().String("spec", "", "Path to the SituationSpec YAML file")
	cmd.Flags().String("trace", "", "Path to the JSONL trace file")
	cmd.Flags().StringVar(&dbPath, "db", "", "SQLite database path (default: <trace>.replay.db)")
	cmd.Flags().StringVar(&tenantID, "tenant", "default", "Tenant ID")

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
