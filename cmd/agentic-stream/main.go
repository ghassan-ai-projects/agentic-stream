// Package main is the entrypoint for the agentic-stream CLI and local runtime.
package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/ghassan-ai-projects/agentic-stream/internal/api"
	"github.com/ghassan-ai-projects/agentic-stream/internal/contractsv1"
	"github.com/ghassan-ai-projects/agentic-stream/internal/evidence"
	"github.com/ghassan-ai-projects/agentic-stream/internal/replay"
	"github.com/ghassan-ai-projects/agentic-stream/internal/runtime"
	"github.com/ghassan-ai-projects/agentic-stream/internal/spec"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
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

	root.AddCommand(newVersionCommand())
	root.AddCommand(newValidateCommand())
	root.AddCommand(newRunCommand())
	root.AddCommand(newConfigEffectiveCommand())
	root.AddCommand(newServeCommand())

	return root
}

func newServeCommand() *cobra.Command {
	var dbPath, listenAddress string
	var ownerLease time.Duration
	cmd := &cobra.Command{
		Use:   "serve",
		Short: "Start the live Go runtime and readiness endpoint.",
		RunE: func(cmd *cobra.Command, _ []string) error {
			if dbPath == "" {
				return fmt.Errorf("--db is required")
			}
			db, err := storage.Open(cmd.Context(), dbPath)
			if err != nil {
				return fmt.Errorf("open runtime database: %w", err)
			}
			defer func() { _ = db.Close() }()
			epoch, err := evidence.NewRuntimeEpoch()
			if err != nil {
				return fmt.Errorf("generate runtime epoch: %w", err)
			}
			owner := &storage.RuntimeOwner{DB: db, InstanceID: epoch, Lease: ownerLease}
			ledger := &evidence.Ledger{DB: db, LeaseOwner: epoch, RuntimeEpoch: epoch, Lease: ownerLease}
			service, err := runtime.NewService(owner, ledger, epoch)
			if err != nil {
				return err
			}
			if _, err := service.Start(cmd.Context()); err != nil {
				return fmt.Errorf("start runtime: %w", err)
			}
			defer func() { _ = service.Close(context.Background()) }()
			server := &http.Server{Addr: listenAddress, Handler: api.NewHealthHandler(service), ReadHeaderTimeout: 5 * time.Second}
			go func() {
				<-cmd.Context().Done()
				shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				_ = server.Shutdown(shutdownCtx)
			}()
			if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				return fmt.Errorf("serve runtime: %w", err)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&dbPath, "db", "", "SQLite runtime database path")
	cmd.Flags().StringVar(&listenAddress, "listen", "127.0.0.1:8080", "loopback HTTP listen address")
	cmd.Flags().DurationVar(&ownerLease, "owner-lease", time.Minute, "runtime owner lease duration")
	return cmd
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
