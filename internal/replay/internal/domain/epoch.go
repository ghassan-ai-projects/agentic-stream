package domain

import (
	"encoding/json"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/contractsv1"
)

// TraceEnvelope decodes one trace line. Malformed lines are not epoch
// evidence; ingress owns their quarantine during ingestion.
func TraceEnvelope(line []byte) (contractsv1.Envelope, bool) {
	if len(line) == 0 {
		return contractsv1.Envelope{}, false
	}
	var envelope contractsv1.Envelope
	if err := json.Unmarshal(line, &envelope); err != nil {
		return contractsv1.Envelope{}, false
	}
	return envelope, true
}

// AdoptTenant fills an empty envelope tenant with the replay tenant before
// contract validation.
func AdoptTenant(envelope contractsv1.Envelope, tenantID string) contractsv1.Envelope {
	if envelope.TenantID == "" {
		envelope.TenantID = tenantID
	}
	return envelope
}

// ContractValidEnvelope reports whether the envelope satisfies the frozen
// envelope contract for the replay tenant.
func ContractValidEnvelope(envelope contractsv1.Envelope, tenantID string) bool {
	return contractsv1.ValidateEnvelope(envelope, tenantID) == nil
}

// EnvelopeProcessingTime selects the processing time a valid line contributes
// to epoch derivation: ingestion time when present, otherwise event time.
func EnvelopeProcessingTime(envelope contractsv1.Envelope) (time.Time, bool) {
	processingTime := envelope.IngestedAt
	if processingTime.IsZero() {
		processingTime = envelope.EventTime
	}
	if processingTime.IsZero() {
		return time.Time{}, false
	}
	return processingTime, true
}

// EpochFromEarliest anchors the virtual clock at the earliest valid trace
// time, or the Unix origin when no valid time exists.
func EpochFromEarliest(earliest time.Time) time.Time {
	if earliest.IsZero() {
		return time.Date(1970, 1, 1, 0, 0, 0, 0, time.UTC)
	}
	return earliest
}

// RecordProcessingTime selects the clock target for one ingested record:
// its ingestion time when present, otherwise its event time, in UTC.
func RecordProcessingTime(ingestedAt, eventTime time.Time) time.Time {
	processingTime := ingestedAt.UTC()
	if processingTime.IsZero() {
		processingTime = eventTime.UTC()
	}
	return processingTime
}
