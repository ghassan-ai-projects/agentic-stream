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
	root.AddCommand(newServeCommand())
	root.AddCommand(newRunLiveCommand())
	root.AddCommand(newExportRunCommand())
	root.AddCommand(newVerifyRunCommand())
	root.AddCommand(newInterlockCommand())
	root.AddCommand(newPrincipalsCommand())
	root.AddCommand(newQuarantineCommand())
	root.AddCommand(newNotificationsCommand())
	root.AddCommand(newCommandsCommand())

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

// runFlags are the replay command's flags.
type runFlags struct {
	dbPath, tenantID string
	repeat           int
}

func newRunCommand() *cobra.Command {
	var flags runFlags
	cmd := &cobra.Command{
		Use:   "run --spec <spec.yaml> --trace <trace.jsonl>",
		Short: "Replay a JSONL trace against a spec and print the canonical result.", Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error { return flags.run(cmd) },
	}
	flags.register(cmd)
	return cmd
}

func (f *runFlags) register(cmd *cobra.Command) {
	cmd.Flags().String("spec", "", "Path to the SituationSpec YAML file")
	cmd.Flags().String("trace", "", "Path to the JSONL trace file")
	cmd.Flags().StringVar(&f.dbPath, "db", "", "SQLite database path (default: <trace>.replay.db)")
	cmd.Flags().StringVar(&f.tenantID, "tenant", "default", "Tenant ID")
	cmd.Flags().IntVar(&f.repeat, "repeat", 1, "Replay N times in fresh databases and fail unless every Situation history hash is identical")
}

func (f *runFlags) run(cmd *cobra.Command) error {
	if f.repeat != 1 {
		return runRepeatedReplay(cmd, f.dbPath, f.tenantID, f.repeat)
	}
	return runReplayCommand(cmd, &f.dbPath, f.tenantID)
}

// runRepeatedReplay is the determinism check: N fresh replays of the same
// trace must produce byte-identical Situation histories.
func runRepeatedReplay(cmd *cobra.Command, dbPath, tenantID string, repeat int) error {
	if repeat < 2 || dbPath != "" {
		return fmt.Errorf("--repeat needs at least 2 runs and its own fresh databases (no --db)")
	}
	specPath, tracePath, err := replayPaths(cmd)
	if err != nil {
		return err
	}
	results, err := replay.RunNTimes(cmd.Context(), replay.Request{SpecPath: specPath, TracePath: tracePath, TenantID: tenantID}, repeat)
	if err != nil {
		return fmt.Errorf("repeat replay: %w", err)
	}
	return reportRepeatedReplay(cmd, results)
}

func reportRepeatedReplay(cmd *cobra.Command, results []replay.Result) error {
	for i, result := range results {
		cmd.Printf("run=%d events_processed=%d situation_versions=%d versions_hash=%s\n", i+1, result.EventsProcessed, result.VersionCount, result.VersionsHash)
	}
	if !replay.AllHashesEqual(results) {
		return fmt.Errorf("replay is not deterministic: the %d runs produced different Situation histories", len(results))
	}
	cmd.Printf("deterministic: %d identical runs\n", len(results))
	return nil
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
