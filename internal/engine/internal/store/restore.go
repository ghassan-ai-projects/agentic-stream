package store

import (
	"time"

	"context"
	"database/sql"
	"fmt"

	"github.com/ghassan-ai-projects/agentic-stream/internal/kernel"

	"github.com/ghassan-ai-projects/agentic-stream/internal/engine/internal/domain"
)

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

// EachCurrentSituation streams every current Situation of the deployment, in
// partition, entity type and entity order, to restore.
func (s Store) EachCurrentSituation(ctx context.Context, restore func(domain.StoredSituation) error) error {
	rows, err := s.db.QueryContext(ctx, selectCurrentSituationsSQL, s.deploymentID, s.tenantID)
	if err != nil {
		return fmt.Errorf("query current situations: %w", err)
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		if err := restoreRow(rows, restore); err != nil {
			return err
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate current situations: %w", err)
	}
	return nil
}

func restoreRow(rows *sql.Rows, restore func(domain.StoredSituation) error) error {
	stored, err := scanStoredSituation(rows)
	if err != nil {
		return err
	}
	return restore(stored)
}

func scanStoredSituation(rows *sql.Rows) (domain.StoredSituation, error) {
	var r domain.StoredSituation
	var previousPhase, traceparent, tracestate sql.NullString
	var firstEventTime, latestEventTime, updatedAt string
	if err := rows.Scan(&r.SituationID, &r.Type, &r.EntityType, &r.EntityID, &r.PartitionID,
		&r.OccurrenceID, &r.Version, &r.Phase, &firstEventTime, &latestEventTime,
		&updatedAt, &r.StateCodecVersion, &r.StateJSON, &r.StateSHA256, &previousPhase, &r.Severity, &r.Confidence,
		&r.Completeness, &traceparent, &tracestate); err != nil {
		return domain.StoredSituation{}, fmt.Errorf("scan current situation: %w", err)
	}
	r.PreviousPhase, r.Traceparent, r.Tracestate = previousPhase.String, traceparent.String, tracestate.String
	if err := parseStoredTimes(&r, firstEventTime, latestEventTime, updatedAt); err != nil {
		return domain.StoredSituation{}, err
	}
	return r, nil
}

func parseStoredTimes(r *domain.StoredSituation, firstEventTime, latestEventTime, updatedAt string) error {
	for _, column := range []struct {
		name, text string
		into       *time.Time
	}{
		{"first event time", firstEventTime, &r.FirstEventTime},
		{"latest event time", latestEventTime, &r.LatestEventTime},
		{"updated time", updatedAt, &r.UpdatedAt},
	} {
		parsed, err := kernel.ParseTime(column.text)
		if err != nil {
			return fmt.Errorf("parse %s of situation %s: %w", column.name, r.SituationID, err)
		}
		*column.into = parsed
	}
	return nil
}
