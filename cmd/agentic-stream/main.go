// Package main is the entrypoint for the agentic-stream CLI and local runtime.
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/ghassan-ai-projects/agentic-stream/internal/contractsv1"
	"github.com/ghassan-ai-projects/agentic-stream/internal/policy"
	"github.com/ghassan-ai-projects/agentic-stream/internal/replay"
	"github.com/ghassan-ai-projects/agentic-stream/internal/spec"
)

// Version metadata injected at build time.
var (
	Version = "dev"
	Commit  = "none"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	root := newRootCommand()
	root.SetContext(ctx)
	if err := root.Execute(); err != nil {
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

	registerCommands(root)

	return root
}

func registerCommands(root *cobra.Command) {
	root.AddCommand(newVersionCommand())
	root.AddCommand(newValidateCommand())
	root.AddCommand(newRunCommand())
	root.AddCommand(newConfigEffectiveCommand())
	root.AddCommand(newServeCommand())
	root.AddCommand(newRunLiveCommand())
	root.AddCommand(newExportRunCommand())
	root.AddCommand(newVerifyRunCommand())
	root.AddCommand(newInterlockCommand())
	root.AddCommand(newPrincipalsCommand())

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
		Use: "validate <spec.yaml>", Short: "Validate and compile a SituationSpec.", Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error { return validateSpecCommand(cmd, args[0], outputJSON) },
	}
	cmd.Flags().BoolVar(&outputJSON, "json", false, "Emit canonical JSON instead of summary")
	return cmd
}

func validateSpecCommand(cmd *cobra.Command, path string, outputJSON bool) error {
	result, err := spec.CompileFile(context.Background(), path)
	if err != nil {
		return fmt.Errorf("compile %s: %w", path, err)
	}
	if outputJSON {
		cmd.Printf("%s\n", result.CanonicalJSON)
		return nil
	}
	return printCompiledSpec(cmd, result)
}

// printCompiledSpec prints the spec summary and the policy digest every device
// command will carry, which a device gateway allow-lists.
func printCompiledSpec(cmd *cobra.Command, result *spec.CompiledSpec) error {
	policyDigest, err := policy.DigestForVersion(result.Digest)
	if err != nil {
		return fmt.Errorf("derive policy digest: %w", err)
	}
	cmd.Printf("ok: %s\n", result.Metadata.Name)
	cmd.Printf("version: %s\n", result.Metadata.Version)
	cmd.Printf("digest: %s\n", result.Digest)
	cmd.Printf("policy_digest: %s\n", policyDigest)
	cmd.Printf("schema: %s\n", result.SchemaVersion)
	return nil
}

func newRunCommand() *cobra.Command {
	var dbPath, tenantID string
	cmd := &cobra.Command{
		Use:   "run --spec <spec.yaml> --trace <trace.jsonl>",
		Short: "Replay a JSONL trace against a spec and print the canonical result.", Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error { return runReplayCommand(cmd, &dbPath, tenantID) },
	}
	cmd.Flags().String("spec", "", "Path to the SituationSpec YAML file")
	cmd.Flags().String("trace", "", "Path to the JSONL trace file")
	cmd.Flags().StringVar(&dbPath, "db", "", "SQLite database path (default: <trace>.replay.db)")
	cmd.Flags().StringVar(&tenantID, "tenant", "default", "Tenant ID")
	return cmd
}

func runReplayCommand(cmd *cobra.Command, dbPath *string, tenantID string) error {
	specPath, tracePath, err := replayPaths(cmd)
	if err != nil {
		return err
	}
	if *dbPath == "" {
		*dbPath = tracePath + ".replay.db"
	}
	result, err := replay.Run(cmd.Context(), replay.Request{DBPath: *dbPath, SpecPath: specPath, TracePath: tracePath, TenantID: tenantID})
	if err != nil {
		return fmt.Errorf("run replay: %w", err)
	}
	cmd.Printf("events_processed=%d situation_versions=%d versions_hash=%s\n", result.EventsProcessed, result.VersionCount, result.VersionsHash)
	return nil
}

func replayPaths(cmd *cobra.Command) (string, string, error) {
	specPath, err := cmd.Flags().GetString("spec")
	if err != nil {
		return "", "", fmt.Errorf("get spec flag: %w", err)
	}
	tracePath, err := cmd.Flags().GetString("trace")
	if err != nil {
		return "", "", fmt.Errorf("get trace flag: %w", err)
	}
	if specPath == "" || tracePath == "" {
		return "", "", fmt.Errorf("--spec and --trace are required")
	}
	return specPath, tracePath, nil
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
