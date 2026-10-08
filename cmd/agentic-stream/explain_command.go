package main

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/ghassan-ai-projects/agentic-stream/internal/cognition"
	"github.com/ghassan-ai-projects/agentic-stream/internal/engine"
	"github.com/ghassan-ai-projects/agentic-stream/internal/eventlog"
	"github.com/ghassan-ai-projects/agentic-stream/internal/spec"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
)

func newExplainCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "explain", Short: "Explain a Situation version or a trigger decision from durable records."}
	cmd.AddCommand(newExplainSituationCommand(), newExplainTriggerCommand())
	return cmd
}

func newExplainSituationCommand() *cobra.Command {
	var flags operatorFlags
	var version int
	cmd := &cobra.Command{
		Use: "situation <situation-id>", Short: "Explain each field of a Situation version, its evidence and its trigger evaluations.", Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return readOperatorDatabase(cmd, flags, func(db *storage.DB) error { return printSituationExplanation(cmd, flags, db, args[0], version) })
		},
	}
	flags.register(cmd)
	cmd.Flags().IntVar(&version, "version", 0, "Situation version (default: current)")
	return cmd
}

func newExplainTriggerCommand() *cobra.Command {
	var flags operatorFlags
	cmd := &cobra.Command{
		Use: "trigger <trigger-id>", Short: "Explain a trigger evaluation and what became of it.", Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return readOperatorDatabase(cmd, flags, func(db *storage.DB) error { return printTriggerExplanation(cmd, flags, db, args[0]) })
		},
	}
	flags.register(cmd)
	return cmd
}

type situationExplanation struct {
	Version  engine.SituationVersionRecord       `json:"version"`
	Fields   []fieldExplanation                  `json:"fields"`
	Evidence []eventlog.EvidenceEvent            `json:"evidence"`
	Triggers []cognition.TriggerEvaluationRecord `json:"triggers"`
}

type fieldExplanation struct {
	Value      any                  `json:"value"`
	Derivation spec.FieldDerivation `json:"derivation"`
}

func printSituationExplanation(cmd *cobra.Command, flags operatorFlags, db *storage.DB, situationID string, version int) error {
	explanation, err := explainSituation(cmd, flags.tenantID, db, situationID, version)
	if err != nil {
		return err
	}
	return printResult(cmd, flags.asJSON, explanation, func() string { return explanation.text() })
}

func explainSituation(cmd *cobra.Command, tenantID string, db *storage.DB, situationID string, version int) (situationExplanation, error) {
	record, err := engine.SituationVersion(cmd.Context(), db, tenantID, situationID, version)
	if err != nil {
		return situationExplanation{}, fmt.Errorf("explain situation: %w", err)
	}
	explanation := situationExplanation{Version: record}
	if explanation.Fields, err = explainFields(cmd, db, record); err != nil {
		return situationExplanation{}, err
	}
	if explanation.Evidence, err = eventlog.NewEventLog(db).EvidenceEvents(cmd.Context(), tenantID, record.Evidence); err != nil {
		return situationExplanation{}, fmt.Errorf("explain situation evidence: %w", err)
	}
	if explanation.Triggers, err = cognition.TriggerEvaluations(cmd.Context(), db.DB, tenantID, situationID, record.Version); err != nil {
		return situationExplanation{}, fmt.Errorf("explain situation triggers: %w", err)
	}
	return explanation, nil
}

func explainFields(cmd *cobra.Command, db *storage.DB, record engine.SituationVersionRecord) ([]fieldExplanation, error) {
	compiled, err := spec.LoadDeployment(cmd.Context(), db, record.DeploymentID)
	if err != nil {
		return nil, fmt.Errorf("explain situation fields: %w", err)
	}
	var snapshot map[string]any
	if err := json.Unmarshal(record.Snapshot, &snapshot); err != nil {
		return nil, fmt.Errorf("decode situation snapshot: %w", err)
	}
	fields := make([]fieldExplanation, 0, len(compiled.FieldDerivations()))
	for _, derivation := range compiled.FieldDerivations() {
		fields = append(fields, fieldExplanation{Value: snapshotValue(snapshot, derivation.Field), Derivation: derivation})
	}
	return fields, nil
}

func snapshotValue(snapshot map[string]any, field string) any {
	if facts, ok := snapshot["facts"].(map[string]any); ok {
		if value, ok := facts[field]; ok {
			return value
		}
	}
	return snapshot[field]
}

func (e situationExplanation) text() string {
	var b strings.Builder
	b.WriteString(versionHeader(e.Version) + "\nfields:\n")
	for _, field := range e.Fields {
		fmt.Fprintf(&b, "  %s = %v  <- %s(%s)%s\n", field.Derivation.Field, compactValue(field.Value), field.Derivation.Strategy, field.Derivation.Input, operatorText(field.Derivation.Operator))
	}
	b.WriteString("evidence:\n")
	for _, event := range e.Evidence {
		fmt.Fprintf(&b, "  #%d %s %s source=%s event_time=%s\n", event.Position, event.EventID, event.EventType, event.Source, event.EventTime)
	}
	b.WriteString("triggers:\n")
	for _, trigger := range e.Triggers {
		fmt.Fprintf(&b, "  %s %s outcome=%s score=%g threshold=%g reasons=%s\n", trigger.TriggerID, trigger.TriggerName, trigger.Outcome, trigger.Score, trigger.Threshold, strings.Join(trigger.Reasons, "; "))
	}
	return strings.TrimSuffix(b.String(), "\n")
}

func operatorText(operator *spec.Operator) string {
	if operator == nil {
		return ""
	}
	parts := []string{" <- operator " + operator.Name + ": " + operator.Kind}
	if operator.Aggregate != "" && operator.Aggregate != operator.Kind {
		parts = append(parts, operator.Aggregate)
	}
	if operator.Field != "" {
		parts = append(parts, "of "+operator.Field)
	}
	if operator.Window != "" {
		parts = append(parts, "over "+operator.Window)
	}
	return strings.Join(parts, " ")
}

func compactValue(value any) string {
	encoded, err := json.Marshal(value)
	if err != nil || len(encoded) > 120 {
		return fmt.Sprintf("<%T>", value)
	}
	return string(encoded)
}
