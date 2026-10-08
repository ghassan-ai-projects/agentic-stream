package main

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/ghassan-ai-projects/agentic-stream/internal/replay"
)

type runFlags struct {
	dbPath, tenantID, sourceDB string
	workerSocket, workerName   string
	repeat                     int
	jsonOutput                 bool
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
	cmd.Flags().StringVar(&f.sourceDB, "source-db", "", "Live runtime database whose recorded decisions every replayed episode must match (opened read-only)")
	cmd.Flags().StringVar(&f.workerSocket, "worker-socket", "", "Shadow mode: candidate EpisodeWorker Unix socket, paired with the deterministic baseline on every replayed episode")
	cmd.Flags().StringVar(&f.workerName, "worker-name", "tamoz", "Shadow mode: expected candidate worker name")
	cmd.Flags().BoolVar(&f.jsonOutput, "json", false, "Shadow mode: print the result, sealed comparisons and both decisions as JSON")
}

func (f *runFlags) run(cmd *cobra.Command) error {
	if err := f.requireOneMode(); err != nil {
		return err
	}
	if f.sourceDB != "" {
		return runRecordedReplay(cmd, f)
	}
	if f.workerSocket != "" {
		return runShadowReplay(cmd, f)
	}
	if f.repeat != 1 {
		return runRepeatedReplay(cmd, f.dbPath, f.tenantID, f.repeat)
	}
	return runReplayCommand(cmd, f)
}

func (f *runFlags) requireOneMode() error {
	selected := 0
	for _, on := range []bool{f.sourceDB != "", f.workerSocket != "", f.repeat != 1} {
		if on {
			selected++
		}
	}
	if selected > 1 {
		return fmt.Errorf("--source-db, --worker-socket and --repeat select different replay modes; use one")
	}
	return nil
}

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

func runRecordedReplay(cmd *cobra.Command, f *runFlags) error {
	request, err := replayRequest(cmd, f)
	if err != nil {
		return err
	}
	result, err := replay.RunRecorded(cmd.Context(), request, f.sourceDB)
	if err != nil {
		return fmt.Errorf("recorded replay: %w", err)
	}
	cmd.Printf("mode=recorded events_processed=%d situation_versions=%d versions_hash=%s recorded_decisions_verified=%d\n", result.EventsProcessed, result.VersionCount, result.VersionsHash, result.CapabilityCalls)
	return nil
}

func replayRequest(cmd *cobra.Command, f *runFlags) (replay.Request, error) {
	specPath, tracePath, err := replayPaths(cmd)
	if err != nil {
		return replay.Request{}, err
	}
	if f.dbPath == "" {
		f.dbPath = tracePath + ".replay.db"
	}
	return replay.Request{DBPath: f.dbPath, SpecPath: specPath, TracePath: tracePath, TenantID: f.tenantID}, nil
}

func runReplayCommand(cmd *cobra.Command, f *runFlags) error {
	request, err := replayRequest(cmd, f)
	if err != nil {
		return err
	}
	result, err := replay.Run(cmd.Context(), request)
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
