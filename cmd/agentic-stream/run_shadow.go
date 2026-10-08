package main

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/ghassan-ai-projects/agentic-stream/internal/replay"
)

func runShadowReplay(cmd *cobra.Command, f *runFlags) error {
	request, err := replayRequest(cmd, f)
	if err != nil {
		return err
	}
	result, err := replay.RunShadow(cmd.Context(), request, f.workerSocket, f.workerName)
	if err != nil {
		return fmt.Errorf("shadow replay: %w", err)
	}
	if f.jsonOutput {
		return printShadowJSON(cmd, result)
	}
	printShadowText(cmd, result)
	return nil
}

func printShadowText(cmd *cobra.Command, result replay.Result) {
	cmd.Printf("mode=shadow events_processed=%d situation_versions=%d versions_hash=%s comparisons=%d differing=%d findings=%d effects_allowed=%t\n", result.EventsProcessed, result.VersionCount, result.VersionsHash, len(result.ShadowComparisons), result.DifferingComparisons(), len(result.Findings), result.EffectsAllowed)
	for _, comparison := range result.ShadowComparisons {
		cmd.Printf("comparison episode=%s decisions_equal=%t comparison_sha256=%s\n", comparison.EpisodeKey, comparison.DecisionsEqual, comparison.ComparisonSHA256)
	}
	for _, finding := range result.Findings {
		cmd.Printf("finding code=%s message=%q\n", finding.Code, finding.Message)
	}
}

func printShadowJSON(cmd *cobra.Command, result replay.Result) error {
	encoded, err := json.MarshalIndent(result.ShadowReport(), "", "  ")
	if err != nil {
		return fmt.Errorf("encode shadow report: %w", err)
	}
	cmd.Println(string(encoded))
	return nil
}
