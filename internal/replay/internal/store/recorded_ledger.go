package store

import (
	"context"
	"database/sql"
	"encoding/hex"
	"fmt"

	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"

	domain "github.com/ghassan-ai-projects/agentic-stream/internal/replay/internal/domain"
)

type SourceLedger struct {
	db                   *sql.DB
	tenantID, specDigest string
}

func NewSourceLedger(db *sql.DB, tenantID, specDigest string) SourceLedger {
	return SourceLedger{db: db, tenantID: tenantID, specDigest: specDigest}
}

func (l SourceLedger) RequireDeployment(ctx context.Context) error {
	var deployed int
	if err := l.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM spec_deployments WHERE deployment_id = ? AND tenant_id = ?", l.specDigest, l.tenantID).Scan(&deployed); err != nil {
		return fmt.Errorf("read source deployments: %w", err)
	}
	if deployed == 0 {
		return fmt.Errorf("source database never deployed spec %s for tenant %s", l.specDigest, l.tenantID)
	}
	return nil
}

const recordedDecisionsQuery = `
	SELECT si.situation_id, si.situation_version, si.trigger_id, e.episode_id, d.attempt_id, d.fence, d.raw_json, d.decision_sha256
	FROM scheduler_items si
	JOIN episodes e ON e.scheduler_item_id = si.scheduler_item_id
	JOIN decisions d ON d.episode_id = e.episode_id AND d.validation_status = 'accepted'
	WHERE si.tenant_id = ?
	AND si.situation_id IN (SELECT situation_id FROM situations WHERE deployment_id = ?)
	ORDER BY si.situation_id, si.situation_version, si.trigger_id`

func (l SourceLedger) Entries(ctx context.Context) ([]domain.RecordedEntry, error) {
	entries, err := storage.QueryAll(ctx, l.db, "recorded decisions", scanRecordedEntry, recordedDecisionsQuery, l.tenantID, l.specDigest)
	if err != nil {
		return nil, fmt.Errorf("read recorded decisions: %w", err)
	}
	return entries, nil
}

func scanRecordedEntry(rows *sql.Rows) (domain.RecordedEntry, error) {
	var entry domain.RecordedEntry
	var attemptID sql.NullString
	var fence sql.NullInt64
	var digest []byte
	if err := rows.Scan(&entry.SituationID, &entry.SituationVersion, &entry.TriggerID, &entry.EpisodeID, &attemptID, &fence, &entry.DecisionJSON, &digest); err != nil {
		return domain.RecordedEntry{}, fmt.Errorf("scan recorded decision: %w", err)
	}
	entry.AttemptID, entry.Fence = attemptID.String, fence.Int64
	entry.DecisionSHA256 = "sha256:" + hex.EncodeToString(digest)
	entry.EpisodeKey = domain.EpisodeKey(entry.SituationID, entry.SituationVersion, entry.TriggerID)
	return withAttemptProvenance(entry)
}

func withAttemptProvenance(entry domain.RecordedEntry) (domain.RecordedEntry, error) {
	provenance, err := domain.AttemptProvenance(entry)
	if err != nil {
		return domain.RecordedEntry{}, fmt.Errorf("attempt provenance of %s: %w", entry.EpisodeKey, err)
	}
	entry.AttemptProvenanceSHA256 = provenance
	return entry, nil
}
