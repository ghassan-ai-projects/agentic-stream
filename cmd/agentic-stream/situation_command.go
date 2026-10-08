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
	var flags operatorFlags
	var entityID string
	cmd := &cobra.Command{
		Use: "list", Short: "List Situations, newest evidence first.", Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return readOperatorDatabase(cmd, flags, func(db *storage.DB) error { return printSituations(cmd, flags, db, entityID) })
		},
	}
	flags.register(cmd)
	cmd.Flags().StringVar(&entityID, "entity", "", "Only Situations of this entity")
	return cmd
}

func newSituationShowCommand() *cobra.Command {
	var flags operatorFlags
	var version int
	cmd := &cobra.Command{
		Use: "show <situation-id>", Short: "Show one Situation version with its snapshot and evidence set.", Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return readOperatorDatabase(cmd, flags, func(db *storage.DB) error { return printSituationVersion(cmd, flags, db, args[0], version) })
		},
	}
	flags.register(cmd)
	cmd.Flags().IntVar(&version, "version", 0, "Situation version (default: current)")
	return cmd
}

func readOperatorDatabase(cmd *cobra.Command, flags operatorFlags, read func(*storage.DB) error) error {
	db, err := flags.openOperatorDatabase(cmd.Context())
	if err != nil {
		return err
	}
	defer func() { _ = db.Close() }()
	return read(db)
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
