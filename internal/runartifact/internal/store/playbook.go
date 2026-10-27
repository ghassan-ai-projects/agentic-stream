package store

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
)

func (s *Snapshot) PlaybookDigests(ctx context.Context, tenantID string) ([]string, error) {
	digests, err := storage.QueryAll(ctx, s.tx, "playbook digests", scanText, `SELECT playbook_digest FROM rule_fires WHERE tenant_id = ?
		UNION SELECT playbook_digest FROM rule_evaluations WHERE tenant_id = ? ORDER BY 1`, tenantID, tenantID)
	if err != nil {
		return nil, fmt.Errorf("read the playbook digests the run recorded: %w", err)
	}
	return digests, nil
}

func scanText(rows *sql.Rows) (string, error) {
	var text string
	if err := rows.Scan(&text); err != nil {
		return "", fmt.Errorf("scan text: %w", err)
	}
	return text, nil
}
