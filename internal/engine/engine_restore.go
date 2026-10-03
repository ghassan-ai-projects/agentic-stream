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
	rows, err := db.QueryContext(ctx, selectCurrentSituationsSQL, deploymentID, tenantID)
	if err != nil {
		return fmt.Errorf("query current situations: %w", err)
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		if err := restoreSituationRow(rows, tenantID, deploymentID, sitEngine); err != nil {
			return err
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate current situations: %w", err)
	}
	return nil
}

const selectCurrentSituationsSQL = `
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
		ORDER BY s.partition_id, s.entity_type, s.entity_id`

func restoreSituationRow(rows *sql.Rows, tenantID, deploymentID string, sitEngine *situations.Engine) error {
	record, err := scanPersistedSituation(rows, tenantID, deploymentID)
	if err != nil {
		return err
	}
	if err := sitEngine.Restore(record.situation); err != nil {
		return fmt.Errorf("restore situation %s: %w", record.situation.SituationID, err)
	}
	return nil
}

type persistedSituationRecord struct {
	situation situations.Situation
}

func scanPersistedSituation(rows *sql.Rows, tenantID, deploymentID string) (persistedSituationRecord, error) {
	row, err := scanSituationRow(rows)
	if err != nil {
		return persistedSituationRecord{}, err
	}
	times, err := row.parseTimes()
	if err != nil {
		return persistedSituationRecord{}, err
	}
	state, err := decodePersistedSituationState(row.situationID, row.occurrenceID, row.partitionID, row.version, row.stateCodecVersion, row.stateJSON, row.stateSHA256)
	if err != nil {
		return persistedSituationRecord{}, err
	}
	return persistedSituationRecord{row.situation(tenantID, deploymentID, times, state)}, nil
}

// situationRow is one current Situation joined with its current version.
type situationRow struct {
	situationID, situationType, entityType, entityID, occurrenceID, phase string
	partitionID, version, severity, stateCodecVersion                     int
	firstEventTime, latestEventTime, updatedAt, completeness              string
	stateJSON, stateSHA256                                                []byte
	previousPhase, traceparent, tracestate                                sql.NullString
	confidence                                                            float64
}

func scanSituationRow(rows *sql.Rows) (situationRow, error) {
	var r situationRow
	if err := rows.Scan(&r.situationID, &r.situationType, &r.entityType, &r.entityID, &r.partitionID,
		&r.occurrenceID, &r.version, &r.phase, &r.firstEventTime, &r.latestEventTime,
		&r.updatedAt, &r.stateCodecVersion, &r.stateJSON, &r.stateSHA256, &r.previousPhase, &r.severity, &r.confidence,
		&r.completeness, &r.traceparent, &r.tracestate); err != nil {
		return situationRow{}, fmt.Errorf("scan current situation: %w", err)
	}
	return r, nil
}

// situationTimes are the parsed lifecycle times of a stored Situation.
type situationTimes struct {
	first, latest, updated time.Time
}

func (r situationRow) parseTimes() (situationTimes, error) {
	var times situationTimes
	var err error
	if times.first, err = parseSituationTime("first event time", r.firstEventTime); err != nil {
		return situationTimes{}, err
	}
	if times.latest, err = parseSituationTime("latest event time", r.latestEventTime); err != nil {
		return situationTimes{}, err
	}
	if times.updated, err = parseSituationTime("updated time", r.updatedAt); err != nil {
		return situationTimes{}, err
	}
	return times, nil
}

func (r situationRow) situation(tenantID, deploymentID string, times situationTimes, state persistedSituationState) situations.Situation {
	return situations.Situation{
		SituationID: r.situationID, TenantID: tenantID, DeploymentID: deploymentID, Type: r.situationType,
		EntityType: r.entityType, EntityID: r.entityID, PartitionID: r.partitionID,
		OccurrenceID: r.occurrenceID, Version: r.version, Phase: r.phase,
		PreviousPhase: r.previousPhase.String, Severity: r.severity, Confidence: r.confidence,
		Completeness: r.completeness, FirstEventTime: times.first, LatestEventTime: times.latest,
		Facts: state.Facts, Evidence: evidenceSet(state.Evidence), ConditionStart: state.ConditionStart,
		OpenedAt: times.first, UpdatedAt: times.updated, Traceparent: r.traceparent.String, Tracestate: r.tracestate.String,
	}
}

func parseSituationTime(name, value string) (time.Time, error) {
	parsed, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return time.Time{}, fmt.Errorf("parse %s: %w", name, err)
	}
	return parsed, nil
}

func decodePersistedSituationState(situationID, occurrenceID string, partitionID, version, codecVersion int, stateJSON, stateSHA256 []byte) (persistedSituationState, error) {
	if err := checkStateCodec(situationID, codecVersion, stateJSON, stateSHA256); err != nil {
		return persistedSituationState{}, err
	}
	if err := verifyStateDigest(situationID, stateJSON, stateSHA256); err != nil {
		return persistedSituationState{}, err
	}
	state, err := decodeState(situationID, stateJSON)
	if err != nil {
		return persistedSituationState{}, err
	}
	if state.SituationID != situationID || state.OccurrenceID != occurrenceID || state.PartitionID != partitionID || state.Version != version {
		return persistedSituationState{}, fmt.Errorf("situation %s persisted state identity mismatch", situationID)
	}
	return state, restoreFactTimes(state.Facts)
}

// checkStateCodec requires codec version 1 and complete state bytes; legacy
// codec 0 state must be rebuilt.
func checkStateCodec(situationID string, codecVersion int, stateJSON, stateSHA256 []byte) error {
	if codecVersion == 0 {
		return fmt.Errorf("situation %s requires rebuild: legacy runtime state has no supported codec", situationID)
	}
	if codecVersion != 1 {
		return fmt.Errorf("situation %s has unsupported state codec %d", situationID, codecVersion)
	}
	if len(stateJSON) == 0 || len(stateSHA256) != sha256.Size {
		return fmt.Errorf("situation %s has incomplete persisted state", situationID)
	}
	return nil
}

func verifyStateDigest(situationID string, stateJSON, stateSHA256 []byte) error {
	var document map[string]any
	if err := json.Unmarshal(stateJSON, &document); err != nil {
		return fmt.Errorf("decode situation state document %s: %w", situationID, err)
	}
	digest, err := canonicaljson.Digest(canonicaljson.DomainSituationState, document)
	if err != nil {
		return fmt.Errorf("digest situation state %s: %w", situationID, err)
	}
	decodedDigest, err := canonicaljson.DecodeDigest(digest)
	if err != nil || !bytes.Equal(decodedDigest, stateSHA256) {
		return fmt.Errorf("situation %s persisted state digest mismatch", situationID)
	}
	return nil
}

func decodeState(situationID string, stateJSON []byte) (persistedSituationState, error) {
	var state persistedSituationState
	if err := json.Unmarshal(stateJSON, &state); err != nil {
		return persistedSituationState{}, fmt.Errorf("decode situation state %s: %w", situationID, err)
	}
	if state.Facts == nil {
		state.Facts = make(map[string]any)
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
