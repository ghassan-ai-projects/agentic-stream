package main

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/ghassan-ai-projects/agentic-stream/internal/runartifact"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
)

func newExportRunCommand() *cobra.Command {
	var dbPath, outputDir string
	var manifest runartifact.Manifest
	cmd := &cobra.Command{
		Use:   "export-run --db <runtime.db> --output <directory>",
		Short: "Export one consistent, verifiable runtime evidence artifact.",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return printExportedArtifact(cmd, dbPath, outputDir, manifest)
		},
	}
	addExportRunFlags(cmd, &dbPath, &outputDir, &manifest)
	return cmd
}

func printExportedArtifact(cmd *cobra.Command, dbPath, outputDir string, manifest runartifact.Manifest) error {
	path, err := exportRunArtifact(cmd.Context(), dbPath, outputDir, manifest)
	if err != nil {
		return err
	}
	cmd.Printf("run_artifact=%s\n", path)
	return nil
}

func exportRunArtifact(ctx context.Context, dbPath, outputDir string, manifest runartifact.Manifest) (string, error) {
	if dbPath == "" || outputDir == "" {
		return "", fmt.Errorf("--db and --output are required")
	}
	db, err := storage.Open(ctx, dbPath)
	if err != nil {
		return "", fmt.Errorf("open runtime database: %w", err)
	}
	defer func() { _ = db.Close() }()
	path, err := runartifact.Export(ctx, runartifact.Options{DB: db, OutputDir: outputDir, Manifest: manifest})
	if err != nil {
		return "", fmt.Errorf("export run artifact: %w", err)
	}
	return path, nil
}

func addExportRunFlags(cmd *cobra.Command, dbPath, outputDir *string, manifest *runartifact.Manifest) {
	cmd.Flags().StringVar(dbPath, "db", "", "SQLite runtime database path")
	cmd.Flags().StringVar(outputDir, "output", "", "New run artifact directory")
	cmd.Flags().IntVar(&manifest.SchemaVersion, "manifest-schema-version", 1, "Run manifest schema version")
	cmd.Flags().StringVar(&manifest.RunID, "run-id", "", "Experiment/run identifier")
	cmd.Flags().StringVar(&manifest.TenantID, "tenant", "", "Tenant identifier")
	cmd.Flags().StringVar(&manifest.GitCommit, "git-commit", Commit, "Agentic Stream git commit")
	cmd.Flags().BoolVar(&manifest.GitDirty, "git-dirty", false, "Whether the source checkout was dirty")
	addDeviceManifestFlags(cmd, &manifest.Device)
	addWorkerManifestFlags(cmd, &manifest.Worker)
	addRunEvidenceFlags(cmd, manifest)
}

func addDeviceManifestFlags(cmd *cobra.Command, device *runartifact.DeviceIdentity) {
	cmd.Flags().StringVar(&device.Board, "board", "", "Board identity")
	cmd.Flags().StringVar(&device.DeviceID, "device-id", "", "Device identity")
	cmd.Flags().StringVar(&device.BootID, "boot-id", "", "Device boot identity")
	cmd.Flags().StringVar(&device.FirmwareDigest, "firmware-digest", "", "Firmware digest")
	cmd.Flags().StringVar(&device.CapabilityDigest, "capability-digest", "", "Capability catalog digest")
}

func addWorkerManifestFlags(cmd *cobra.Command, worker *runartifact.WorkerMetadata) {
	cmd.Flags().StringVar(&worker.PromptVersion, "prompt-version", "", "Worker prompt version")
	cmd.Flags().StringVar(&worker.PromptDigest, "prompt-digest", "", "Worker prompt digest")
	cmd.Flags().StringVar(&worker.DecisionSchema, "decision-schema", "", "Worker decision schema")
	cmd.Flags().StringVar(&worker.Provider, "provider", "", "Worker provider")
	cmd.Flags().StringVar(&worker.Model, "model", "", "Worker model")
	cmd.Flags().StringVar(&worker.SamplingParameters, "sampling-parameters", "", "Worker sampling parameters")
}

func addRunEvidenceFlags(cmd *cobra.Command, manifest *runartifact.Manifest) {
	cmd.Flags().StringVar(&manifest.SpecDigest, "spec-digest", "", "SituationSpec digest")
	cmd.Flags().StringVar(&manifest.PolicyDigest, "policy-digest", "", "Policy digest")
	cmd.Flags().StringVar(&manifest.CalibrationRevision, "calibration-revision", "", "Calibration revision")
	cmd.Flags().StringVar(&manifest.WiringRevision, "wiring-revision", "", "Wiring revision")
	cmd.Flags().StringVar(&manifest.ScenarioSeed, "scenario-seed", "", "Scenario seed")
	cmd.Flags().StringVar(&manifest.WallClockStart, "start", "", "Run start timestamp")
	cmd.Flags().StringVar(&manifest.WallClockEnd, "end", "", "Run end timestamp")
	cmd.Flags().Int64Var(&manifest.MonotonicStartUS, "monotonic-start-us", 0, "Monotonic start timestamp")
	cmd.Flags().Int64Var(&manifest.MonotonicEndUS, "monotonic-end-us", 0, "Monotonic end timestamp")
	cmd.Flags().StringVar(&manifest.OperatorIdentity, "operator", "", "Operator identity")
	cmd.Flags().StringVar(&manifest.SafetyReviewReference, "safety-review-reference", "", "Safety review reference")
	cmd.Flags().StringVar(&manifest.DeclaredResult, "declared-result", "", "Operator-declared result")
}

func newVerifyRunCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "verify-run <directory>",
		Short: "Verify a previously exported run artifact.",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			if err := runartifact.Verify(args[0]); err != nil {
				return fmt.Errorf("verify run artifact: %w", err)
			}
			return nil
		},
	}
}
