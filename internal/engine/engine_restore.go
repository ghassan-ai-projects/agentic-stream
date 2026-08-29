package engine

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/canonicaljson"
	"github.com/ghassan-ai-projects/agentic-stream/internal/situations"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
)

type persistedSituationState struct {
	SituationID    string               `json:"situation_id"`
	OccurrenceID   string               `json:"occurrence_id"`
	PartitionID    int                  `json:"partition_id"`
	Version        int                  `json:"version"`
	Facts          map[string]any       `json:"facts"`
	Evidence       []string             `json:"evidence"`
	ConditionStart map[string]time.Time `json:"condition_start"`
	Traceparent    string               `json:"traceparent,omitempty"`
	Tracestate     string               `json:"tracestate,omitempty"`
}

func restoreSituations(ctx context.Context, db *storage.DB, deploymentID, tenantID string, sitEngine *situations.Engine) error {
	rows, err := db.QueryContext(ctx, `
		SELECT s.situation_id, s.situation_type, s.entity_type, s.entity_id, s.partition_id,
		       s.occurrence_id, s.current_version, s.phase,
		       s.first_event_time, s.latest_event_time, s.updated_at,
		       s.state_codec_version, s.state_json, s.state_sha256,
		       v.previous_phase, v.severity, v.confidence,
		       v.completeness, v.traceparent, v.tracestate
		FROM situations s
		JOIN situation_versions v
		  ON v.situation_id = s.situation_id AND v.version = s.current_version
		WHERE s.deployment_id = ? AND s.tenant_id = ?
		ORDER BY s.partition_id, s.entity_type, s.entity_id`, deploymentID, tenantID)
	if err != nil {
		return fmt.Errorf("query current situations: %w", err)
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		record, err := scanPersistedSituation(rows, tenantID, deploymentID)
		if err != nil {
			return err
		}
		if err := sitEngine.Restore(record.situation); err != nil {
			return fmt.Errorf("restore situation %s: %w", record.situation.SituationID, err)
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate current situations: %w", err)
	}
	return nil
}

type persistedSituationRecord struct {
	situation situations.Situation
}

func scanPersistedSituation(rows *sql.Rows, tenantID, deploymentID string) (persistedSituationRecord, error) {
	var (
		situationID, situationType, entityType, entityID, occurrenceID, phase string
		partitionID, version, severity                                        int
		firstEventTime, latestEventTime, updatedAt                            string
		stateCodecVersion                                                     int
		stateJSON, stateSHA256                                                []byte
		previousPhase, traceparent, tracestate                                sql.NullString
		confidence                                                            float64
		completeness                                                          string
	)
	if err := rows.Scan(&situationID, &situationType, &entityType, &entityID, &partitionID,
		&occurrenceID, &version, &phase, &firstEventTime, &latestEventTime,
		&updatedAt, &stateCodecVersion, &stateJSON, &stateSHA256, &previousPhase, &severity, &confidence,
		&completeness, &traceparent, &tracestate); err != nil {
		return persistedSituationRecord{}, fmt.Errorf("scan current situation: %w", err)
	}
	first, err := parseSituationTime("first event time", firstEventTime)
	if err != nil {
		return persistedSituationRecord{}, err
	}
	latest, err := parseSituationTime("latest event time", latestEventTime)
	if err != nil {
		return persistedSituationRecord{}, err
	}
	updated, err := parseSituationTime("updated time", updatedAt)
	if err != nil {
		return persistedSituationRecord{}, err
	}
	state, err := decodePersistedSituationState(situationID, occurrenceID, partitionID, version, stateCodecVersion, stateJSON, stateSHA256)
	if err != nil {
		return persistedSituationRecord{}, err
	}
	return persistedSituationRecord{situations.Situation{
		SituationID: situationID, TenantID: tenantID, DeploymentID: deploymentID, Type: situationType,
		EntityType: entityType, EntityID: entityID, PartitionID: partitionID,
		OccurrenceID: occurrenceID, Version: version, Phase: phase,
		PreviousPhase: previousPhase.String, Severity: severity, Confidence: confidence,
		Completeness: completeness, FirstEventTime: first, LatestEventTime: latest,
		Facts: state.Facts, Evidence: evidenceSet(state.Evidence), ConditionStart: state.ConditionStart,
		OpenedAt: first, UpdatedAt: updated, Traceparent: traceparent.String, Tracestate: tracestate.String,
	}}, nil
}

func parseSituationTime(name, value string) (time.Time, error) {
	parsed, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return time.Time{}, fmt.Errorf("parse %s: %w", name, err)
	}
	return parsed, nil
}

func decodePersistedSituationState(situationID, occurrenceID string, partitionID, version, codecVersion int, stateJSON, stateSHA256 []byte) (persistedSituationState, error) {
	if codecVersion == 0 {
		return persistedSituationState{}, fmt.Errorf("situation %s requires rebuild: legacy runtime state has no supported codec", situationID)
	}
	if codecVersion != 1 {
		return persistedSituationState{}, fmt.Errorf("situation %s has unsupported state codec %d", situationID, codecVersion)
	}
	if len(stateJSON) == 0 || len(stateSHA256) != sha256.Size {
		return persistedSituationState{}, fmt.Errorf("situation %s has incomplete persisted state", situationID)
	}
	var document map[string]any
	if err := json.Unmarshal(stateJSON, &document); err != nil {
		return persistedSituationState{}, fmt.Errorf("decode situation state document %s: %w", situationID, err)
	}
	digest, err := canonicaljson.Digest(canonicaljson.DomainSituationState, document)
	if err != nil {
		return persistedSituationState{}, fmt.Errorf("digest situation state %s: %w", situationID, err)
	}
	decodedDigest, err := canonicaljson.DecodeDigest(digest)
	if err != nil || !bytes.Equal(decodedDigest, stateSHA256) {
		return persistedSituationState{}, fmt.Errorf("situation %s persisted state digest mismatch", situationID)
	}
	var state persistedSituationState
	if err := json.Unmarshal(stateJSON, &state); err != nil {
		return persistedSituationState{}, fmt.Errorf("decode situation state %s: %w", situationID, err)
	}
	if state.SituationID != situationID || state.OccurrenceID != occurrenceID || state.PartitionID != partitionID || state.Version != version {
		return persistedSituationState{}, fmt.Errorf("situation %s persisted state identity mismatch", situationID)
	}
	if state.Facts == nil {
		state.Facts = make(map[string]any)
	}
	if err := restoreFactTimes(state.Facts); err != nil {
		return persistedSituationState{}, err
	}
	return state, nil
}

func restoreFactTimes(facts map[string]any) error {
	for key, value := range facts {
		if !strings.HasSuffix(key, "_event_time") {
			continue
		}
		text, ok := value.(string)
		if !ok {
			continue
		}
		parsed, err := time.Parse(time.RFC3339Nano, text)
		if err != nil {
			return fmt.Errorf("parse fact time %s: %w", key, err)
		}
		facts[key] = parsed
	}
	return nil
}

func evidenceSet(ids []string) map[string]struct{} {
	evidence := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		evidence[id] = struct{}{}
	}
	return evidence
}
