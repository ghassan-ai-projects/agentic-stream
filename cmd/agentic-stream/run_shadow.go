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
	cmd.Printf("mode=shadow events_processed=%d situation_versions=%d versions_hash=%s comparisons=%d differing=%d findings=%d effects_allowed=%t\n", result.EventsProcessed, result.VersionCount, result.VersionsHash, len(result.ShadowComparisons), differingComparisons(result), len(result.Findings), result.EffectsAllowed)
	for _, comparison := range result.ShadowComparisons {
		cmd.Printf("comparison episode=%s decisions_equal=%t comparison_sha256=%s\n", comparison.EpisodeKey, comparison.DecisionsEqual, comparison.ComparisonSHA256)
	}
	for _, finding := range result.Findings {
		cmd.Printf("finding code=%s message=%q\n", finding.Code, finding.Message)
	}
}

func differingComparisons(result replay.Result) int {
	differing := 0
	for _, comparison := range result.ShadowComparisons {
		if !comparison.DecisionsEqual {
			differing++
		}
	}
	return differing
}

type shadowReport struct {
	Mode            string             `json:"mode"`
	EventsProcessed int                `json:"events_processed"`
	VersionCount    int                `json:"situation_versions"`
	VersionsHash    string             `json:"versions_hash"`
	EffectsAllowed  bool               `json:"effects_allowed"`
	Comparisons     []shadowComparison `json:"comparisons"`
	Findings        []shadowFinding    `json:"findings"`
}

type shadowComparison struct {
	EpisodeKey        string          `json:"episode_key"`
	ComparisonSHA256  string          `json:"comparison_sha256"`
	DecisionsEqual    bool            `json:"decisions_equal"`
	Comparison        json.RawMessage `json:"comparison"`
	BaselineDecision  json.RawMessage `json:"baseline_decision"`
	CandidateDecision json.RawMessage `json:"candidate_decision"`
}

type shadowFinding struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func printShadowJSON(cmd *cobra.Command, result replay.Result) error {
	encoded, err := json.MarshalIndent(newShadowReport(result), "", "  ")
	if err != nil {
		return fmt.Errorf("encode shadow report: %w", err)
	}
	cmd.Println(string(encoded))
	return nil
}

func newShadowReport(result replay.Result) shadowReport {
	report := shadowReport{Mode: string(result.Mode), EventsProcessed: result.EventsProcessed, VersionCount: result.VersionCount, VersionsHash: result.VersionsHash, EffectsAllowed: result.EffectsAllowed, Comparisons: []shadowComparison{}, Findings: []shadowFinding{}}
	for _, c := range result.ShadowComparisons {
		report.Comparisons = append(report.Comparisons, shadowComparison{EpisodeKey: c.EpisodeKey, ComparisonSHA256: c.ComparisonSHA256, DecisionsEqual: c.DecisionsEqual, Comparison: c.ComparisonJSON, BaselineDecision: c.BaselineDecisionJSON, CandidateDecision: c.TamozDecisionJSON})
	}
	for _, finding := range result.Findings {
		report.Findings = append(report.Findings, shadowFinding{Code: finding.Code, Message: finding.Message})
	}
	return report
}
