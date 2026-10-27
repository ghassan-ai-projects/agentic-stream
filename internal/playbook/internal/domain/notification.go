package domain

import (
	"fmt"
	"strconv"

	"github.com/ghassan-ai-projects/agentic-stream/internal/contractsv1"
	"github.com/ghassan-ai-projects/agentic-stream/internal/kernel"
)

const (
	circuitOpenedType   = "playbook.rule.circuit_opened"
	circuitOpenedSchema = "urn:situation-runtime:schema:rule-circuit:v1"
)

func CircuitOpenedEvent(circuit Circuit, source string) (contractsv1.CloudEvent, error) {
	subject := "rule/" + circuit.Rule.ID + "/" + strconv.Itoa(circuit.Rule.Version)
	event := contractsv1.CloudEvent{
		SpecVersion: "1.0", ID: subject + ":" + circuit.Cause + ":" + kernel.FormatTime(circuit.OpenedAt), Source: source,
		Type: circuitOpenedType, Subject: subject, Time: circuit.OpenedAt, DataContentType: "application/json",
		DataSchema: circuitOpenedSchema, TenantID: circuit.TenantID, PartitionKey: subject, IngestedTime: circuit.OpenedAt,
		Classification: contractsv1.ClassificationInternal,
		Data:           map[string]any{"rule_id": circuit.Rule.ID, "rule_version": circuit.Rule.Version, "cause": circuit.Cause, "evidence": circuit.Evidence},
	}
	digest, err := event.ComputeEnvelopeDigest()
	if err != nil {
		return contractsv1.CloudEvent{}, fmt.Errorf("digest the circuit notification of rule %s: %w", circuit.Rule.ID, err)
	}
	event.EnvelopeDigest = digest
	return event, nil
}
