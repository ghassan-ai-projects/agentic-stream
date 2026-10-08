package domain

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/canonicaljson"
	"github.com/ghassan-ai-projects/agentic-stream/internal/contractsv1"
	"github.com/ghassan-ai-projects/agentic-stream/internal/sources"
)

func (e *Engine) materialize(sit *Situation, watermark time.Time) (*Version, error) {
	if err := e.checkSpecDigest(); err != nil {
		return nil, err
	}
	version := publicationVersion(sit, watermark)
	snapshotJSON, snapshotDigest, err := e.snapshot(sit, version.Facts, version.Evidence, watermark)
	if err != nil {
		return nil, err
	}
	stateBlob, stateDigest, err := persistedState(sit)
	if err != nil {
		return nil, err
	}
	version.SnapshotJSON, version.SnapshotSHA256 = snapshotJSON, snapshotDigest
	version.StateJSON, version.StateSHA256 = stateBlob, stateDigest
	return version, nil
}

func (e *Engine) checkSpecDigest() error {
	if e.spec.Digest == "" {
		return fmt.Errorf("compiled spec has no digest")
	}
	if _, err := canonicaljson.DecodeDigest(e.spec.Digest); err != nil {
		return fmt.Errorf("invalid spec digest: %w", err)
	}
	return nil
}

func publicationVersion(sit *Situation, watermark time.Time) *Version {
	return &Version{
		SituationID: sit.SituationID, Type: sit.Type, Version: sit.Version, PreviousVersion: sit.Version - 1,
		Phase: sit.Phase, PreviousPhase: sit.PreviousPhase, Severity: sit.Severity, Confidence: sit.Confidence,
		Completeness: sit.Completeness, EntityType: sit.EntityType, EntityID: sit.EntityID,
		EventHorizon: sit.LatestEventTime, Watermark: watermark,
		Facts: publishedFacts(sit), Evidence: sortedEvidenceIDs(sit),
		Traceparent: sit.Traceparent, Tracestate: sit.Tracestate,
		OccurrenceID: sit.OccurrenceID, FirstEventTime: sit.FirstEventTime, UpdatedAt: sit.UpdatedAt,
		ConditionStart: cloneTimes(sit.ConditionStart),
	}
}

// publishedFacts are the situation facts without internal event-time
// bookkeeping.
func publishedFacts(sit *Situation) map[string]any {
	facts := make(map[string]any, len(sit.Facts))
	for k, v := range sit.Facts {
		if !strings.HasSuffix(k, "_event_time") {
			facts[k] = v
		}
	}
	return facts
}

func sortedEvidenceIDs(sit *Situation) []string {
	evidenceIDs := make([]string, 0, len(sit.Evidence))
	for id := range sit.Evidence {
		evidenceIDs = append(evidenceIDs, id)
	}
	sort.Strings(evidenceIDs)
	return evidenceIDs
}

// snapshot builds the immutable, schema-valid Situation snapshot and returns
// its canonical JSON and digest.
func (e *Engine) snapshot(sit *Situation, facts map[string]any, evidenceIDs []string, watermark time.Time) ([]byte, string, error) {
	snapshot := e.snapshotDocument(sit, facts, evidenceIDs, watermark)
	if err := contractsv1.Validate(contractsv1.SchemaSnapshot, snapshot); err != nil {
		return nil, "", fmt.Errorf("validate snapshot: %w", err)
	}
	snapshotJSON, err := canonicaljson.Marshal(snapshot)
	if err != nil {
		return nil, "", fmt.Errorf("marshal snapshot: %w", err)
	}
	digest, err := canonicaljson.Digest(canonicaljson.DomainSnapshot, snapshot)
	if err != nil {
		return nil, "", fmt.Errorf("digest snapshot: %w", err)
	}
	return snapshotJSON, digest, nil
}

func (e *Engine) snapshotDocument(sit *Situation, facts map[string]any, evidenceIDs []string, watermark time.Time) map[string]any {
	evidence := make([]any, len(evidenceIDs))
	for i, id := range evidenceIDs {
		evidence[i] = id
	}
	return map[string]any{
		"situation_id": sit.SituationID, "situation_version": sit.Version, "situation_type": sit.Type,
		"tenant_id": sit.TenantID, "entity": map[string]any{"type": sit.EntityType, "id": sit.EntityID},
		"partition_id": sit.PartitionID, "phase": sit.Phase, "previous_phase": sit.PreviousPhase,
		"severity": sit.Severity, "confidence": sit.Confidence, "completeness": sit.Completeness,
		"facts": facts, "evidence": evidence,
		"event_horizon": sit.LatestEventTime.Format(time.RFC3339Nano),
		"watermark":     watermark.Format(time.RFC3339Nano), "spec_digest": e.spec.Digest,
	}
}

// persistedState returns the situation's persisted runtime state and its
// digest.
func persistedState(sit *Situation) ([]byte, string, error) {
	stateBlob, err := stateJSON(sit)
	if err != nil {
		return nil, "", fmt.Errorf("marshal persisted state: %w", err)
	}
	var stateDocument map[string]any
	if err := json.Unmarshal(stateBlob, &stateDocument); err != nil {
		return nil, "", fmt.Errorf("decode persisted state: %w", err)
	}
	stateDigest, err := canonicaljson.Digest(canonicaljson.DomainSituationState, stateDocument)
	if err != nil {
		return nil, "", fmt.Errorf("digest persisted state: %w", err)
	}
	return stateBlob, stateDigest, nil
}

func stateJSON(sit *Situation) ([]byte, error) {
	blob, err := canonicaljson.Marshal(stateDocument(sit, stateFacts(sit), sortedEvidenceIDs(sit), stateConditionStart(sit)))
	if err != nil {
		return nil, fmt.Errorf("marshal situation state: %w", err)
	}
	return blob, nil
}

// stateFacts renders time-valued facts as RFC 3339 text.
func stateFacts(sit *Situation) map[string]any {
	facts := make(map[string]any, len(sit.Facts))
	for key, value := range sit.Facts {
		if timestamp, ok := value.(time.Time); ok {
			facts[key] = sources.FormatTime(timestamp)
			continue
		}
		facts[key] = value
	}
	return facts
}

func stateConditionStart(sit *Situation) map[string]string {
	conditionStart := make(map[string]string, len(sit.ConditionStart))
	for key, value := range sit.ConditionStart {
		conditionStart[key] = sources.FormatTime(value)
	}
	return conditionStart
}

func stateDocument(sit *Situation, facts map[string]any, evidence []string, conditionStart map[string]string) map[string]any {
	return map[string]any{
		"situation_id":    sit.SituationID,
		"occurrence_id":   sit.OccurrenceID,
		"partition_id":    sit.PartitionID,
		"version":         sit.Version,
		"facts":           facts,
		"evidence":        evidence,
		"condition_start": conditionStart,
		"traceparent":     sit.Traceparent,
		"tracestate":      sit.Tracestate,
	}
}

func cloneTimes(values map[string]time.Time) map[string]time.Time {
	clone := make(map[string]time.Time, len(values))
	for key, value := range values {
		clone[key] = value
	}
	return clone
}
