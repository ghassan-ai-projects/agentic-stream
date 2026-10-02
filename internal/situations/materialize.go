package situations

import (
	"encoding/json"
	"fmt"
	"github.com/ghassan-ai-projects/agentic-stream/internal/canonicaljson"
	"github.com/ghassan-ai-projects/agentic-stream/internal/contractsv1"
	"sort"
	"strings"
	"time"
)

func (e *Engine) materialize(sit *Situation, watermark time.Time) (*Version, error) {
	facts := make(map[string]any, len(sit.Facts))
	for k, v := range sit.Facts {
		if !strings.HasSuffix(k, "_event_time") {
			facts[k] = v
		}
	}

	evidenceIDs := make([]string, 0, len(sit.Evidence))
	for id := range sit.Evidence {
		evidenceIDs = append(evidenceIDs, id)
	}
	sort.Strings(evidenceIDs)
	evidence := make([]any, len(evidenceIDs))
	for i, id := range evidenceIDs {
		evidence[i] = id
	}

	specDigest := e.spec.Digest
	if specDigest == "" {
		return nil, fmt.Errorf("compiled spec has no digest")
	}
	if _, err := canonicaljson.DecodeDigest(specDigest); err != nil {
		return nil, fmt.Errorf("invalid spec digest: %w", err)
	}
	snapshot := map[string]any{
		"situation_id":      sit.SituationID,
		"situation_version": sit.Version,
		"situation_type":    sit.Type,
		"tenant_id":         sit.TenantID,
		"entity":            map[string]any{"type": sit.EntityType, "id": sit.EntityID},
		"partition_id":      sit.PartitionID,
		"phase":             sit.Phase,
		"previous_phase":    sit.PreviousPhase,
		"severity":          sit.Severity,
		"confidence":        sit.Confidence,
		"completeness":      sit.Completeness,
		"facts":             facts,
		"evidence":          evidence,
		"event_horizon":     sit.LatestEventTime.Format(time.RFC3339Nano),
		"watermark":         watermark.Format(time.RFC3339Nano),
		"spec_digest":       specDigest,
	}
	if err := contractsv1.Validate(contractsv1.SchemaSnapshot, snapshot); err != nil {
		return nil, fmt.Errorf("validate snapshot: %w", err)
	}
	snapshotJSON, err := canonicaljson.Marshal(snapshot)
	if err != nil {
		return nil, fmt.Errorf("marshal snapshot: %w", err)
	}
	digest, err := canonicaljson.Digest(canonicaljson.DomainSnapshot, snapshot)
	if err != nil {
		return nil, fmt.Errorf("digest snapshot: %w", err)
	}
	stateBlob, err := stateJSON(sit)
	if err != nil {
		return nil, fmt.Errorf("marshal persisted state: %w", err)
	}
	var stateDocument map[string]any
	if err := json.Unmarshal(stateBlob, &stateDocument); err != nil {
		return nil, fmt.Errorf("decode persisted state: %w", err)
	}
	stateDigest, err := canonicaljson.Digest(canonicaljson.DomainSituationState, stateDocument)
	if err != nil {
		return nil, fmt.Errorf("digest persisted state: %w", err)
	}

	return &Version{
		SituationID:     sit.SituationID,
		Type:            sit.Type,
		Version:         sit.Version,
		PreviousVersion: sit.Version - 1,
		Phase:           sit.Phase,
		PreviousPhase:   sit.PreviousPhase,
		Severity:        sit.Severity,
		Confidence:      sit.Confidence,
		Completeness:    sit.Completeness,
		EntityType:      sit.EntityType,
		EntityID:        sit.EntityID,
		EventHorizon:    sit.LatestEventTime,
		Watermark:       watermark,
		Facts:           facts,
		Evidence:        evidenceIDs,
		SnapshotJSON:    snapshotJSON,
		SnapshotSHA256:  digest,
		Traceparent:     sit.Traceparent,
		Tracestate:      sit.Tracestate,
		OccurrenceID:    sit.OccurrenceID,
		FirstEventTime:  sit.FirstEventTime,
		UpdatedAt:       sit.UpdatedAt,
		ConditionStart:  cloneTimes(sit.ConditionStart),
		StateJSON:       stateBlob,
		StateSHA256:     stateDigest,
	}, nil
}

func stateJSON(sit *Situation) ([]byte, error) {
	evidence := make([]string, 0, len(sit.Evidence))
	for id := range sit.Evidence {
		evidence = append(evidence, id)
	}
	sort.Strings(evidence)
	facts := make(map[string]any, len(sit.Facts))
	for key, value := range sit.Facts {
		if timestamp, ok := value.(time.Time); ok {
			facts[key] = timestamp.UTC().Format(time.RFC3339Nano)
			continue
		}
		facts[key] = value
	}
	conditionStart := make(map[string]string, len(sit.ConditionStart))
	for key, value := range sit.ConditionStart {
		conditionStart[key] = value.UTC().Format(time.RFC3339Nano)
	}
	blob, err := canonicaljson.Marshal(stateDocument(sit, facts, evidence, conditionStart))
	if err != nil {
		return nil, fmt.Errorf("marshal situation state: %w", err)
	}
	return blob, nil
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
