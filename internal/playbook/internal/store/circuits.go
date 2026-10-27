package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/kernel"
	"github.com/ghassan-ai-projects/agentic-stream/internal/notify"
	"github.com/ghassan-ai-projects/agentic-stream/internal/playbook/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
)

func (t *Tx) OpenCircuits(ctx context.Context, tenantID string) (map[domain.RuleKey]bool, error) {
	keys, err := storage.QueryAll(ctx, t.tx, "open circuits", scanRuleKey,
		`SELECT rule_id, rule_version FROM rule_circuits WHERE tenant_id = ? AND state = 'open'`, tenantID)
	if err != nil {
		return nil, fmt.Errorf("read open rule circuits: %w", err)
	}
	open := make(map[domain.RuleKey]bool, len(keys))
	for _, key := range keys {
		open[key] = true
	}
	return open, nil
}

func scanRuleKey(rows *sql.Rows) (domain.RuleKey, error) {
	var key domain.RuleKey
	if err := rows.Scan(&key.ID, &key.Version); err != nil {
		return domain.RuleKey{}, fmt.Errorf("scan rule key: %w", err)
	}
	return key, nil
}

func (t *Tx) OpenCircuit(ctx context.Context, circuit domain.Circuit) error {
	evidence, err := json.Marshal(circuit.Evidence)
	if err != nil {
		return fmt.Errorf("encode the evidence of the %s circuit: %w", circuit.Rule.ID, err)
	}
	result, err := t.tx.ExecContext(ctx, `INSERT INTO rule_circuits (rule_id, rule_version, tenant_id, state, cause, evidence_json, opened_at)
		VALUES (?, ?, ?, 'open', ?, ?, ?)
		ON CONFLICT (rule_id, rule_version) DO UPDATE SET state = 'open', cause = excluded.cause,
			evidence_json = excluded.evidence_json, opened_at = excluded.opened_at, closed_at = NULL, close_reason = NULL
		WHERE rule_circuits.state = 'closed'`,
		circuit.Rule.ID, circuit.Rule.Version, circuit.TenantID, circuit.Cause, evidence, kernel.FormatTime(circuit.OpenedAt))
	if err != nil {
		return fmt.Errorf("open the circuit of rule %s: %w", circuit.Rule.ID, err)
	}
	return t.announceOpened(ctx, circuit, result)
}

func (t *Tx) announceOpened(ctx context.Context, circuit domain.Circuit, result sql.Result) error {
	opened, err := storage.RowsAffected(result)
	if err != nil {
		return fmt.Errorf("open the circuit of rule %s: %w", circuit.Rule.ID, err)
	}
	if opened == 0 {
		return nil
	}
	event, err := domain.CircuitOpenedEvent(circuit, notify.SourceForTenant(circuit.TenantID))
	if err != nil {
		return fmt.Errorf("announce the open circuit of rule %s: %w", circuit.Rule.ID, err)
	}
	if _, err := notify.Append(ctx, t.tx, event, circuit.OpenedAt); err != nil {
		return fmt.Errorf("announce the open circuit of rule %s: %w", circuit.Rule.ID, err)
	}
	return nil
}

func (t *Tx) Circuits(ctx context.Context, tenantID string) ([]domain.CircuitView, error) {
	circuits, err := storage.QueryAll(ctx, t.tx, "rule circuits", scanCircuit, `SELECT rule_id, rule_version, state, cause, opened_at,
		COALESCE(closed_at, ''), COALESCE(close_reason, '') FROM rule_circuits WHERE tenant_id = ? ORDER BY opened_at, rule_id, rule_version`, tenantID)
	if err != nil {
		return nil, fmt.Errorf("read rule circuits: %w", err)
	}
	return circuits, nil
}

func scanCircuit(rows *sql.Rows) (domain.CircuitView, error) {
	var c domain.CircuitView
	if err := rows.Scan(&c.Rule.ID, &c.Rule.Version, &c.State, &c.Cause, &c.OpenedAt, &c.ClosedAt, &c.CloseReason); err != nil {
		return domain.CircuitView{}, fmt.Errorf("scan rule circuit: %w", err)
	}
	return c, nil
}

func (t *Tx) CloseCircuit(ctx context.Context, tenantID string, rule domain.RuleKey, reason string, now time.Time) error {
	result, err := t.tx.ExecContext(ctx, `UPDATE rule_circuits SET state = 'closed', closed_at = ?, close_reason = ?
		WHERE tenant_id = ? AND rule_id = ? AND rule_version = ? AND state = 'open'`, kernel.FormatTime(now), reason, tenantID, rule.ID, rule.Version)
	if err != nil {
		return fmt.Errorf("close the circuit of rule %s v%d: %w", rule.ID, rule.Version, err)
	}
	closed, err := storage.RowsAffected(result)
	if err != nil {
		return fmt.Errorf("close the circuit of rule %s v%d: %w", rule.ID, rule.Version, err)
	}
	if closed == 0 {
		return fmt.Errorf("%w: rule %s v%d", domain.ErrNoOpenCircuit, rule.ID, rule.Version)
	}
	return nil
}
