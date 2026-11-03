package domain

import (
	"fmt"
	"strconv"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/contractsv1"
	"github.com/ghassan-ai-projects/agentic-stream/internal/kernel"
)

const (
	circuitOpenedType   = "playbook.rule.circuit_opened"
	circuitOpenedSchema = "urn:situation-runtime:schema:rule-circuit:v1"
)

type notice struct {
	id, eventType, schema, subject, source, tenantID string
	at                                               time.Time
	data                                             map[string]any
}

func CircuitOpenedEvent(circuit Circuit, source string) (contractsv1.CloudEvent, error) {
	subject := "rule/" + circuit.Rule.ID + "/" + strconv.Itoa(circuit.Rule.Version)
	return sealed(notice{id: subject + ":" + circuit.Cause + ":" + kernel.FormatTime(circuit.OpenedAt), eventType: circuitOpenedType,
		schema: circuitOpenedSchema, subject: subject, source: source, tenantID: circuit.TenantID, at: circuit.OpenedAt,
		data: map[string]any{"rule_id": circuit.Rule.ID, "rule_version": circuit.Rule.Version, "cause": circuit.Cause, "evidence": circuit.Evidence}})
}

func ProposedEvent(proposal Proposal, source string, now time.Time) (contractsv1.CloudEvent, error) {
	subject := "proposal/" + proposal.ProposalID
	return sealed(notice{id: subject, eventType: proposedEventType, schema: proposedSchema, subject: subject, source: source,
		tenantID: proposal.TenantID, at: now,
		data: map[string]any{"proposal_id": proposal.ProposalID, "proposal_digest": proposal.Digest, "key": proposal.Key,
			"risk_class": proposal.RiskClass, "rule": proposal.Rule, "replay": proposal.Replay, "evidence_cases": proposal.EvidenceCases}})
}

func sealed(n notice) (contractsv1.CloudEvent, error) {
	event := contractsv1.CloudEvent{
		SpecVersion: "1.0", ID: n.id, Source: n.source, Type: n.eventType, Subject: n.subject, Time: n.at,
		DataContentType: "application/json", DataSchema: n.schema, TenantID: n.tenantID, PartitionKey: n.subject,
		IngestedTime: n.at, Classification: contractsv1.ClassificationInternal, Data: n.data,
	}
	digest, err := event.ComputeEnvelopeDigest()
	if err != nil {
		return contractsv1.CloudEvent{}, fmt.Errorf("digest the %s notification %s: %w", n.eventType, n.id, err)
	}
	event.EnvelopeDigest = digest
	return event, nil
}
