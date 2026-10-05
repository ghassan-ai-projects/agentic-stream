package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/cognition/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/contractsv1"
	"github.com/ghassan-ai-projects/agentic-stream/internal/situations"
)

func (t *Tx) LoadLastReasonedVersion(ctx context.Context, situationID string) (int, error) {
	var last int
	if err := t.tx.QueryRowContext(ctx,
		"SELECT last_reasoned_version FROM situations WHERE situation_id = ?",
		situationID,
	).Scan(&last); err != nil {
		if err == sql.ErrNoRows {
			return 0, nil
		}
		return 0, fmt.Errorf("query last reasoned version: %w", err)
	}
	return last, nil
}

func (t *Tx) LoadVersion(ctx context.Context, situationID string, version int) (*situations.Version, error) {
	if !domain.HasPreviousVersion(version) {
		return nil, nil
	}
	row, found, err := queryVersionRow(ctx, t.tx, situationID, version)
	if err != nil || !found {
		return nil, err
	}
	return row.Version()
}

// versionRow is one stored Situation version with its entity, as scanned.
type versionRow struct {
	v                       situations.Version
	eventHorizon, watermark string
	Traceparent, Tracestate sql.NullString
	snapshotJSON            []byte
}

func queryVersionRow(ctx context.Context, tx *sql.Tx, situationID string, version int) (versionRow, bool, error) {
	var r versionRow
	err := tx.QueryRowContext(ctx, selectVersionSQL, situationID, version).Scan(
		&r.v.SituationID, &r.v.Version, &r.v.Phase, &r.v.PreviousPhase,
		&r.v.Severity, &r.v.Confidence, &r.v.Completeness,
		&r.v.EntityType, &r.v.EntityID, &r.eventHorizon, &r.watermark, &r.Traceparent, &r.Tracestate, &r.snapshotJSON,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return versionRow{}, false, nil
	}
	if err != nil {
		return versionRow{}, false, fmt.Errorf("query version: %w", err)
	}
	return r, true, nil
}

const selectVersionSQL = `
		SELECT sv.situation_id, sv.version, sv.phase, sv.previous_phase, sv.severity,
		       sv.confidence, sv.completeness, s.entity_type, s.entity_id,
		       sv.event_horizon, sv.watermark, sv.traceparent, sv.tracestate, sv.snapshot_json
		FROM situation_versions sv
		JOIN situations s ON s.situation_id = sv.situation_id
		WHERE sv.situation_id = ? AND sv.version = ?`

// version parses the row's times, validates its trace context and recovers
// the snapshot facts.
func (r versionRow) Version() (*situations.Version, error) {
	v := r.v
	var err error
	if v.EventHorizon, err = time.Parse(time.RFC3339Nano, r.eventHorizon); err != nil {
		return nil, fmt.Errorf("parse event horizon: %w", err)
	}
	if v.Traceparent, v.Tracestate, err = r.traceContext(); err != nil {
		return nil, err
	}
	if v.Watermark, err = parseOptionalTime(r.watermark); err != nil {
		return nil, fmt.Errorf("parse watermark: %w", err)
	}
	if v.Facts, err = snapshotFacts(r.snapshotJSON); err != nil {
		return nil, err
	}
	return &v, nil
}

func (r versionRow) traceContext() (string, string, error) {
	if _, err := contractsv1.ParseTraceContext(r.Traceparent.String, r.Tracestate.String); err != nil {
		return "", "", fmt.Errorf("validate version trace context: %w", err)
	}
	return r.Traceparent.String, r.Tracestate.String, nil
}

func parseOptionalTime(value string) (time.Time, error) {
	if value == "" {
		return time.Time{}, nil
	}
	return time.Parse(time.RFC3339Nano, value) //nolint:wrapcheck // The caller names the field.
}

// snapshotFacts returns the snapshot's facts object, or nil when it has none.
func snapshotFacts(snapshotJSON []byte) (map[string]any, error) {
	var snapshot map[string]any
	if err := json.Unmarshal(snapshotJSON, &snapshot); err != nil {
		return nil, fmt.Errorf("unmarshal snapshot: %w", err)
	}
	facts, _ := snapshot["facts"].(map[string]any)
	return facts, nil
}
