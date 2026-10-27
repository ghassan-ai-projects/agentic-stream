package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/kernel"
	"github.com/ghassan-ai-projects/agentic-stream/internal/playbook/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
)

func (t *Tx) RecordFire(ctx context.Context, fire domain.Fire) error {
	recommendation, err := json.Marshal(fire.Recommendation)
	if err != nil {
		return fmt.Errorf("encode the recommendation of fire %s: %w", fire.FireID, err)
	}
	if _, err := t.tx.ExecContext(ctx, `INSERT INTO rule_fires (fire_id, tenant_id, rule_id, rule_version, playbook_digest, episode_id,
		situation_id, situation_version, recommendation_json, audit_due, event_time) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		fire.FireID, fire.TenantID, fire.Recommendation.RuleID, fire.Recommendation.RuleVersion, fire.Recommendation.PlaybookDigest,
		fire.EpisodeID, fire.SituationID, fire.SituationVersion, recommendation, storage.BoolInt(fire.AuditDue),
		kernel.FormatTime(fire.EventTime)); err != nil {
		return fmt.Errorf("record fire %s: %w", fire.FireID, err)
	}
	return nil
}

func (t *Tx) FiresSince(ctx context.Context, tenantID string, since time.Time) (map[domain.RuleKey]int, error) {
	counts, err := storage.QueryAll(ctx, t.tx, "rule fires", scanFireCount, `SELECT rule_id, rule_version, COUNT(*) FROM rule_fires
		WHERE tenant_id = ? AND event_time >= ? GROUP BY rule_id, rule_version`, tenantID, kernel.FormatTime(since))
	if err != nil {
		return nil, fmt.Errorf("count rule fires since %s: %w", kernel.FormatTime(since), err)
	}
	fires := make(map[domain.RuleKey]int, len(counts))
	for _, count := range counts {
		fires[count.key] = count.fires
	}
	return fires, nil
}

type fireCount struct {
	key   domain.RuleKey
	fires int
}

func scanFireCount(rows *sql.Rows) (fireCount, error) {
	var count fireCount
	if err := rows.Scan(&count.key.ID, &count.key.Version, &count.fires); err != nil {
		return fireCount{}, fmt.Errorf("scan rule fire count: %w", err)
	}
	return count, nil
}

func (t *Tx) DueAudits(ctx context.Context, tenantID string) ([]domain.Fire, error) {
	fires, err := storage.QueryAll(ctx, t.tx, "due audits", scanFire, `SELECT fire_id, tenant_id, episode_id, situation_id,
		situation_version, recommendation_json, audit_due, event_time FROM rule_fires
		WHERE tenant_id = ? AND audit_due = 1 AND audit_queued = 0 ORDER BY event_time, fire_id`, tenantID)
	if err != nil {
		return nil, fmt.Errorf("read due audits: %w", err)
	}
	return fires, nil
}

func scanFire(rows *sql.Rows) (domain.Fire, error) {
	var fire domain.Fire
	var recommendation []byte
	var auditDue int
	var eventTime string
	if err := rows.Scan(&fire.FireID, &fire.TenantID, &fire.EpisodeID, &fire.SituationID, &fire.SituationVersion,
		&recommendation, &auditDue, &eventTime); err != nil {
		return domain.Fire{}, fmt.Errorf("scan rule fire: %w", err)
	}
	if err := json.Unmarshal(recommendation, &fire.Recommendation); err != nil {
		return domain.Fire{}, fmt.Errorf("decode the recommendation of fire %s: %w", fire.FireID, err)
	}
	fire.AuditDue = auditDue == 1
	return fire, parseInto(&fire.EventTime, eventTime)
}

func parseInto(target *time.Time, text string) error {
	parsed, err := kernel.ParseStoredTime(text)
	if err != nil {
		return fmt.Errorf("parse stored time %q: %w", text, err)
	}
	*target = parsed
	return nil
}

func (t *Tx) MarkAuditQueued(ctx context.Context, fireID string) error {
	if _, err := t.tx.ExecContext(ctx, `UPDATE rule_fires SET audit_queued = 1 WHERE fire_id = ?`, fireID); err != nil {
		return fmt.Errorf("mark the audit of fire %s queued: %w", fireID, err)
	}
	return nil
}
