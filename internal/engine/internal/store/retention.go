package store

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/engine/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/kernel"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
)

type Retention struct {
	tx       *sql.Tx
	tenantID string
}

func JoinRetention(tx *sql.Tx, tenantID string) Retention {
	return Retention{tx: tx, tenantID: tenantID}
}

type pruneStep struct {
	what  string
	query string
	into  *int64
}

func pruneSteps(report *domain.PruneReport) []pruneStep {
	return []pruneStep{
		{"applied inbox entries", pruneInboxSQL, &report.InboxEntries},
		{"finished timers", pruneTimersSQL, &report.Timers},
		{"superseded situation versions", pruneSupersededVersionsSQL, &report.SituationVersions},
		{"unreferenced lineage sets", pruneLineageSQL, &report.LineageSets},
	}
}

func (r Retention) Prune(ctx context.Context, before time.Time) (domain.PruneReport, error) {
	cutoff := kernel.FormatTime(before)
	var report domain.PruneReport
	for _, step := range pruneSteps(&report) {
		removed, err := r.remove(ctx, step.query, cutoff)
		if err != nil {
			return report, fmt.Errorf("prune %s: %w", step.what, err)
		}
		*step.into = removed
	}
	return report, nil
}

func (r Retention) remove(ctx context.Context, query, cutoff string) (int64, error) {
	result, err := r.tx.ExecContext(ctx, query, sql.Named("tenant", r.tenantID), sql.Named("before", cutoff))
	if err != nil {
		return 0, fmt.Errorf("delete: %w", err)
	}
	return storage.RowsAffected(result) //nolint:wrapcheck // storage.RowsAffected wraps its own failure.
}

const pruneInboxSQL = `
		DELETE FROM event_inbox
		WHERE consumer_name = 'engine' AND tenant_id = :tenant AND applied_at < :before`

const pruneTimersSQL = `
		DELETE FROM timers
		WHERE tenant_id = :tenant AND status IN ('fired', 'cancelled') AND created_at < :before`

const pruneSupersededVersionsSQL = `
		DELETE FROM situation_versions
		WHERE created_at < :before
		  AND EXISTS (
		      SELECT 1 FROM situations s
		      WHERE s.situation_id = situation_versions.situation_id AND s.tenant_id = :tenant
		        AND situation_versions.version < s.current_version
		        AND situation_versions.version NOT IN (s.last_reasoned_version, s.last_material_version))
		  AND NOT EXISTS (SELECT 1 FROM trigger_evaluations t WHERE t.situation_id = situation_versions.situation_id AND t.situation_version = situation_versions.version)
		  AND NOT EXISTS (SELECT 1 FROM scheduler_items i WHERE i.situation_id = situation_versions.situation_id AND i.situation_version = situation_versions.version)
		  AND NOT EXISTS (SELECT 1 FROM episodes e WHERE e.situation_id = situation_versions.situation_id AND e.situation_version = situation_versions.version)
		  AND NOT EXISTS (SELECT 1 FROM decisions d WHERE d.situation_id = situation_versions.situation_id AND d.situation_version = situation_versions.version)
		  AND NOT EXISTS (SELECT 1 FROM intents n WHERE n.situation_id = situation_versions.situation_id AND n.situation_version = situation_versions.version)`

const pruneLineageSQL = `
		DELETE FROM lineage_sets
		WHERE created_at < :before
		  AND NOT EXISTS (SELECT 1 FROM situation_versions v WHERE v.lineage_id = lineage_sets.lineage_id)`
