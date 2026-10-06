package store

import (
	"context"
	"database/sql"
	"fmt"

	domain "github.com/ghassan-ai-projects/agentic-stream/internal/runartifact/internal/domain"
)

type ledgerQuery struct {
	query  string
	tenant bool
}

var ledgerQueries = map[string]ledgerQuery{
	domain.FileObservations: {"SELECT * FROM event_log WHERE tenant_id = ? ORDER BY position", true},
	domain.FileSituations: {`SELECT sv.* FROM situation_versions sv
			JOIN situations s ON s.situation_id = sv.situation_id
			WHERE s.tenant_id = ? ORDER BY sv.situation_id, sv.version`, true},
	domain.FileDecisions: {`SELECT d.* FROM decisions d
			JOIN episodes e ON e.episode_id = d.episode_id
			WHERE e.tenant_id = ? ORDER BY d.decision_id`, true},
	domain.FileCommands: {"SELECT * FROM commands WHERE tenant_id = ? ORDER BY command_id", true},
	domain.FileResults: {`SELECT o.* FROM outcomes o
			JOIN commands c ON c.command_id = o.command_id
			WHERE c.tenant_id = ? ORDER BY o.command_id, o.ordinal`, true},
	domain.FileFeedback: {`SELECT v.* FROM verifications v
			JOIN commands c ON c.command_id = v.command_id
			WHERE c.tenant_id = ? ORDER BY v.verification_id`, true},
	domain.FileBindings: {`SELECT b.* FROM device_command_bindings b
			JOIN commands c ON c.command_id = b.command_id
			WHERE c.tenant_id = ? ORDER BY b.command_id`, true},
	domain.FileAuthority: {"SELECT * FROM device_authority_events ORDER BY event_id", false},
	domain.FileSafety:    {"SELECT * FROM device_safety_events ORDER BY event_id", false},
}

// Ledger reads one exported ledger file's rows. The authority and safety
// ledgers are global because the device tables carry no tenant column.
func (s *Snapshot) Ledger(ctx context.Context, file, tenantID string) (domain.LedgerTable, error) {
	item, ok := ledgerQueries[file]
	if !ok {
		return domain.LedgerTable{}, fmt.Errorf("unknown ledger file %q", file)
	}
	var args []any
	if item.tenant {
		args = []any{tenantID}
	}
	return s.table(ctx, item.query, args)
}

func (s *Snapshot) table(ctx context.Context, query string, args []any) (domain.LedgerTable, error) {
	rows, err := s.tx.QueryContext(ctx, query, args...)
	if err != nil {
		return domain.LedgerTable{}, fmt.Errorf("query artifact rows: %w", err)
	}
	defer func() { _ = rows.Close() }()
	columns, err := rows.Columns()
	if err != nil {
		return domain.LedgerTable{}, fmt.Errorf("read artifact columns: %w", err)
	}
	table := domain.LedgerTable{Columns: columns}
	if table.Rows, err = scanRows(rows, len(columns)); err != nil {
		return domain.LedgerTable{}, err
	}
	return table, nil
}

func scanRows(rows *sql.Rows, width int) ([][]any, error) {
	var out [][]any
	for rows.Next() {
		values, err := scanRow(rows, width)
		if err != nil {
			return nil, err
		}
		out = append(out, values)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate artifact rows: %w", err)
	}
	return out, nil
}

func scanRow(rows *sql.Rows, width int) ([]any, error) {
	values := make([]any, width)
	pointers := make([]any, width)
	for i := range values {
		pointers[i] = &values[i]
	}
	if err := rows.Scan(pointers...); err != nil {
		return nil, fmt.Errorf("scan artifact row: %w", err)
	}
	return values, nil
}
