package main

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/ghassan-ai-projects/agentic-stream/internal/engine"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
)

func newSituationCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "situation", Short: "List Situations and show one version."}
	cmd.AddCommand(newSituationListCommand(), newSituationShowCommand())
	return cmd
}

func newSituationListCommand() *cobra.Command {
	var entityID string
	return newDatabaseCommand("list", "List Situations, newest evidence first.", cobra.NoArgs,
		func(cmd *cobra.Command, flags operatorFlags, db *storage.DB, _ []string) error {
			return printSituations(cmd, flags, db, entityID)
		},
		func(cmd *cobra.Command) {
			cmd.Flags().StringVar(&entityID, "entity", "", "Only Situations of this entity")
		})
}

func newSituationShowCommand() *cobra.Command {
	return newVersionedCommand("show <situation-id>", "Show one Situation version with its snapshot and evidence set.", printSituationVersion)
}

func printSituations(cmd *cobra.Command, flags operatorFlags, db *storage.DB, entityID string) error {
	situations, err := engine.ListSituations(cmd.Context(), db, flags.tenantID, entityID)
	if err != nil {
		return fmt.Errorf("list situations: %w", err)
	}
	return printResult(cmd, flags.asJSON, situations, func() string { return situationTable(situations) })
}

func situationTable(situations []engine.SituationSummary) string {
	if len(situations) == 0 {
		return "situations: none"
	}
	var b strings.Builder
	for _, s := range situations {
		fmt.Fprintf(&b, "%s type=%s entity=%s/%s version=%d material=%d phase=%s status=%s latest_event=%s\n", s.SituationID, s.SituationType, s.EntityType, s.EntityID, s.CurrentVersion, s.LastMaterialVersion, s.Phase, s.Status, s.LatestEventTime)
	}
	return strings.TrimSuffix(b.String(), "\n")
}

func printSituationVersion(cmd *cobra.Command, flags operatorFlags, db *storage.DB, situationID string, version int) error {
	record, err := engine.SituationVersion(cmd.Context(), db, flags.tenantID, situationID, version)
	if err != nil {
		return fmt.Errorf("show situation: %w", err)
	}
	return printResult(cmd, flags.asJSON, record, func() string { return versionHeader(record) + "\nsnapshot: " + string(record.Snapshot) })
}

func versionHeader(v engine.SituationVersionRecord) string {
	return fmt.Sprintf("%s version=%d phase=%s (from %s) severity=%d confidence=%g completeness=%s event_horizon=%s watermark=%s snapshot=%s evidence=%d",
		v.SituationID, v.Version, v.Phase, orNone(v.PreviousPhase), v.Severity, v.Confidence, v.Completeness, v.EventHorizon, orNone(v.Watermark), v.SnapshotSHA256, len(v.Evidence))
}

func orNone(value string) string {
	if value == "" {
		return "none"
	}
	return value
}
