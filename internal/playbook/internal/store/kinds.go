package store

import (
	"context"
	"database/sql"
	"fmt"
	"maps"
	"slices"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/kernel"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
)

func (t *Tx) RecordKindDigests(ctx context.Context, tenantID string, born map[string]string, now time.Time) error {
	for _, kind := range slices.Sorted(maps.Keys(born)) {
		if _, err := t.tx.ExecContext(ctx, `INSERT INTO rule_kind_digests (tenant_id, kind, digest, recorded_at) VALUES (?, ?, ?, ?)
			ON CONFLICT (tenant_id, kind) DO UPDATE SET digest = excluded.digest, recorded_at = excluded.recorded_at
			WHERE rule_kind_digests.digest != excluded.digest`, tenantID, kind, born[kind], kernel.FormatTime(now)); err != nil {
			return fmt.Errorf("record the born digest of kind %s: %w", kind, err)
		}
	}
	return nil
}

func (t *Tx) KindDigests(ctx context.Context, tenantID string) (map[string]string, error) {
	rows, err := storage.QueryAll(ctx, t.tx, "kind digests", scanKindDigest, `SELECT kind, digest FROM rule_kind_digests WHERE tenant_id = ?`, tenantID)
	if err != nil {
		return nil, fmt.Errorf("read the born kind digests: %w", err)
	}
	digests := make(map[string]string, len(rows))
	for _, row := range rows {
		digests[row[0]] = row[1]
	}
	return digests, nil
}

func scanKindDigest(rows *sql.Rows) ([2]string, error) {
	var row [2]string
	if err := rows.Scan(&row[0], &row[1]); err != nil {
		return row, fmt.Errorf("scan a kind digest: %w", err)
	}
	return row, nil
}
