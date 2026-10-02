package replay

import (
	"context"
	"database/sql"
	"fmt"
	"github.com/ghassan-ai-projects/agentic-stream/internal/episodes"
	"github.com/ghassan-ai-projects/agentic-stream/internal/ids"
	"github.com/ghassan-ai-projects/agentic-stream/internal/spec"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
	"time"
)

func materializeReplayEpisodes(ctx context.Context, db *storage.DB, compiled *spec.CompiledSpec, tenantID string, now time.Time) error {
	assembler := episodes.NewAssembler(compiled, ids.Deterministic())
	if err := db.WithTx(ctx, func(tx *sql.Tx) error {
		executable, err := executableSchedulerItems(ctx, tx, tenantID, now)
		if err != nil {
			return err
		}
		for _, item := range executable {
			req, err := assembler.Assemble(ctx, tx, item.id, tenantID)
			if err != nil {
				return fmt.Errorf("assemble scheduler item %s: %w", item.id, err)
			}
			if err := assembler.Persist(ctx, tx, req, item.admitAt); err != nil {
				return fmt.Errorf("persist replay episode %s: %w", req.EpisodeID, err)
			}
		}
		return nil
	}); err != nil {
		return fmt.Errorf("materialize replay episodes: %w", err)
	}
	return nil
}

type executableItem struct {
	id      string
	admitAt time.Time
}

// executableSchedulerItems lists pending scheduler items whose admission time
// (creation, or a later not-before) has arrived by now and precedes expiry.
func executableSchedulerItems(ctx context.Context, tx *sql.Tx, tenantID string, now time.Time) ([]executableItem, error) {
	rows, err := tx.QueryContext(ctx, `
		SELECT scheduler_item_id, created_at, not_before, expires_at
		FROM scheduler_items
		WHERE tenant_id = ? AND status = 'pending'
		ORDER BY scheduler_item_id`, tenantID)
	if err != nil {
		return nil, fmt.Errorf("query executable scheduler items: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var executable []executableItem
	for rows.Next() {
		var itemID, createdAt, expiresAt string
		var notBefore sql.NullString
		if err := rows.Scan(&itemID, &createdAt, &notBefore, &expiresAt); err != nil {
			return nil, fmt.Errorf("scan executable scheduler item: %w", err)
		}
		admitAt, expires, err := admissionWindow(createdAt, notBefore, expiresAt)
		if err != nil {
			return nil, err
		}
		if expires.After(admitAt) && !admitAt.After(now) {
			executable = append(executable, executableItem{id: itemID, admitAt: admitAt})
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate executable scheduler items: %w", err)
	}
	return executable, nil
}

// admissionWindow parses when a scheduler item may first be admitted and when
// it expires.
func admissionWindow(createdAt string, notBefore sql.NullString, expiresAt string) (time.Time, time.Time, error) {
	admitAt, err := time.Parse(time.RFC3339Nano, createdAt)
	if err != nil {
		return time.Time{}, time.Time{}, fmt.Errorf("parse scheduler creation time: %w", err)
	}
	expires, err := time.Parse(time.RFC3339Nano, expiresAt)
	if err != nil {
		return time.Time{}, time.Time{}, fmt.Errorf("parse scheduler expiry: %w", err)
	}
	if notBefore.Valid {
		notBeforeTime, err := time.Parse(time.RFC3339Nano, notBefore.String)
		if err != nil {
			return time.Time{}, time.Time{}, fmt.Errorf("parse scheduler not-before: %w", err)
		}
		if notBeforeTime.After(admitAt) {
			admitAt = notBeforeTime
		}
	}
	return admitAt, expires, nil
}
