package store

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/episodes"
	"github.com/ghassan-ai-projects/agentic-stream/internal/ids"
	domain "github.com/ghassan-ai-projects/agentic-stream/internal/replay/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/spec"
)

type executableItem struct {
	id      string
	admitAt time.Time
}

// MaterializeEpisodes turns every executable scheduler item into a durable
// episode through the episodes module's assembler, in one transaction.
func (s Store) MaterializeEpisodes(ctx context.Context, compiled *spec.CompiledSpec, tenantID string, now time.Time) error {
	assembler, err := episodes.New(episodes.Config{Spec: compiled, IDGenerator: ids.Deterministic()})
	if err != nil {
		return fmt.Errorf("configure replay episodes: %w", err)
	}
	if err := s.DB.WithTx(ctx, func(tx *sql.Tx) error {
		return persistExecutableEpisodes(ctx, tx, assembler, tenantID, now)
	}); err != nil {
		return fmt.Errorf("materialize replay episodes: %w", err)
	}
	return nil
}

func persistExecutableEpisodes(ctx context.Context, tx *sql.Tx, assembler *episodes.Service, tenantID string, now time.Time) error {
	executable, err := executableSchedulerItems(ctx, tx, tenantID, now)
	if err != nil {
		return err
	}
	for _, item := range executable {
		if err := persistReplayEpisode(ctx, tx, assembler, item, tenantID); err != nil {
			return err
		}
	}
	return nil
}

func persistReplayEpisode(ctx context.Context, tx *sql.Tx, assembler *episodes.Service, item executableItem, tenantID string) error {
	req, err := assembler.Assemble(ctx, tx, item.id, tenantID)
	if err != nil {
		return fmt.Errorf("assemble scheduler item %s: %w", item.id, err)
	}
	if err := assembler.Persist(ctx, tx, req, item.admitAt); err != nil {
		return fmt.Errorf("persist replay episode %s: %w", req.EpisodeID, err)
	}
	return nil
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
	return collectExecutableItems(rows, now)
}

func collectExecutableItems(rows *sql.Rows, now time.Time) ([]executableItem, error) {
	var executable []executableItem
	for rows.Next() {
		item, eligible, err := executableSchedulerItem(rows, now)
		if err != nil {
			return nil, err
		}
		if eligible {
			executable = append(executable, item)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate executable scheduler items: %w", err)
	}
	return executable, nil
}

func executableSchedulerItem(rows *sql.Rows, now time.Time) (executableItem, bool, error) {
	var itemID, createdAt, expiresAt string
	var notBefore sql.NullString
	if err := rows.Scan(&itemID, &createdAt, &notBefore, &expiresAt); err != nil {
		return executableItem{}, false, fmt.Errorf("scan executable scheduler item: %w", err)
	}
	admitAt, expires, err := domain.AdmissionWindow(createdAt, notBefore.String, expiresAt)
	if err != nil {
		return executableItem{}, false, err
	}
	return executableItem{id: itemID, admitAt: admitAt}, domain.AdmissionReady(admitAt, expires, now), nil
}
