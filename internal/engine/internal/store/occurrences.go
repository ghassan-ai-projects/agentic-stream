package store

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/kernel"
	"github.com/ghassan-ai-projects/agentic-stream/internal/situations"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
)

type Occurrences struct{ tx *sql.Tx }

func JoinOccurrences(tx *sql.Tx) Occurrences { return Occurrences{tx: tx} }

func (o Occurrences) ResolvedAfter(ctx context.Context, situationID string, version int) (time.Time, bool, error) {
	return o.optionalTime(ctx, `SELECT event_horizon FROM situation_versions WHERE situation_id = ? AND phase = ?
		AND version > ? ORDER BY version LIMIT 1`, situationID, situations.PhaseResolved, version)
}

func (o Occurrences) ReopenedAfter(ctx context.Context, tenantID, entityID, situationType string, after time.Time) (time.Time, bool, error) {
	return o.optionalTime(ctx, `SELECT sv.event_horizon FROM situations s JOIN situation_versions sv ON sv.situation_id = s.situation_id
		WHERE s.tenant_id = ? AND s.entity_id = ? AND s.situation_type = ? AND sv.phase != ? AND sv.event_horizon > ?
		ORDER BY sv.event_horizon LIMIT 1`, tenantID, entityID, situationType, situations.PhaseResolved, kernel.FormatTime(after))
}

func (o Occurrences) optionalTime(ctx context.Context, query string, args ...any) (time.Time, bool, error) {
	text, found, err := storage.QueryOptional[sql.NullString](ctx, o.tx, query, args...)
	if err != nil {
		return time.Time{}, false, fmt.Errorf("read a situation occurrence time: %w", err)
	}
	if !found || !text.Valid {
		return time.Time{}, false, nil
	}
	at, err := kernel.ParseStoredTime(text.String)
	if err != nil {
		return time.Time{}, false, fmt.Errorf("parse a situation occurrence time %q: %w", text.String, err)
	}
	return at, true, nil
}
