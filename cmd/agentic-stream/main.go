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
	"github.com/ghassan-ai-projects/agentic-stream/internal/spec"
)

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
	var logging logFlags
	root := &cobra.Command{
		Use:   "agentic-stream",
		Short: "Streaming-native agent runtime (Situation Runtime).",
		Long: `Agentic Stream continuously converts unbounded evidence into durable,
versioned Situations and starts bounded agent episodes only when a deterministic
cognitive scheduler decides reasoning is useful.`,
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	logging.register(root)
	registerEventSchemasFlag(root)
	registerCommands(root)

	return root
}

func registerCommands(root *cobra.Command) {
	root.AddCommand(newVersionCommand(), newValidateCommand(), newRunCommand(), newServeCommand(), newRunLiveCommand(),
		newExportRunCommand(), newVerifyRunCommand())
	root.AddCommand(newInterlockCommand(), newPrincipalsCommand(), newQuarantineCommand(), newNotificationsCommand(),
		newCommandsCommand(), newSituationCommand(), newExplainCommand(), newEpisodeCommand(), newIntentCommand(),
		newMaintenanceCommand())
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
	var playbookPath string
	cmd := &cobra.Command{
		Use: "validate <spec.yaml>", Short: "Validate and compile a SituationSpec and, optionally, its playbook.", Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return validateSpecCommand(cmd, args[0], playbookPath, outputJSON)
		},
	}
	cmd.Flags().BoolVar(&outputJSON, "json", false, "Emit canonical JSON instead of summary")
	cmd.Flags().StringVar(&playbookPath, "playbook", "", "playbook YAML path compiled against the spec; prints its own digest")
	return cmd
}

func validateSpecCommand(cmd *cobra.Command, path, playbookPath string, outputJSON bool) error {
	schemas, err := loadEventSchemas(cmd)
	if err != nil {
		return err
	}
	result, err := spec.CompileFile(commandContext(cmd), path, spec.WithEventSchemas(schemas))
	if err != nil {
		return fmt.Errorf("compile %s: %w", path, err)
	}
	if outputJSON {
		cmd.Printf("%s\n", result.CanonicalJSON)
		return nil
	}
	return printValidation(cmd, result, playbookPath)
}

func printValidation(cmd *cobra.Command, result *spec.CompiledSpec, playbookPath string) error {
	if err := printCompiledSpec(cmd, result); err != nil {
		return err
	}
	return printCompiledPlaybook(cmd, result, playbookPath)
}

func printCompiledPlaybook(cmd *cobra.Command, compiled *spec.CompiledSpec, path string) error {
	if path == "" {
		return nil
	}
	playbook, err := spec.CompilePlaybookFile(compiled, path)
	if err != nil {
		return fmt.Errorf("validate playbook: %w", err)
	}
	cmd.Printf("playbook: %s\n", playbook.Metadata.Name)
	cmd.Printf("playbook_rules: %d\n", len(playbook.Rules))
	cmd.Printf("playbook_digest: %s\n", playbook.Digest)
	return nil
}

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

func commandContext(cmd *cobra.Command) context.Context {
	if ctx := cmd.Context(); ctx != nil {
		return ctx
	}
	return context.Background()
}
