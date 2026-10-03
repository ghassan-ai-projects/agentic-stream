package runartifact

import (
	"context"
	"database/sql"
	"fmt"
)

func exportLedgerFiles(ctx context.Context, tx *sql.Tx, tenantID string) (map[string][]byte, error) {
	queries := ledgerQueries(tenantID)
	files := make(map[string][]byte, len(queries))
	for name, item := range queries {
		data, err := queryJSONL(ctx, tx, item.query, item.args...)
		if err != nil {
			return nil, fmt.Errorf("export %s: %w", name, err)
		}
		files[name] = data
	}
	return files, nil
}

func ledgerQueries(tenantID string) map[string]ledgerQuery {
	return map[string]ledgerQuery{
		"observations.jsonl":            {"SELECT * FROM event_log WHERE tenant_id = ? ORDER BY position", []any{tenantID}},
		"situations.jsonl":              {situationsQuery, []any{tenantID}},
		"decisions.jsonl":               {decisionsQuery, []any{tenantID}},
		"commands.jsonl":                {"SELECT * FROM commands WHERE tenant_id = ? ORDER BY command_id", []any{tenantID}},
		"device-results.jsonl":          {deviceResultsQuery, []any{tenantID}},
		"feedback.jsonl":                {feedbackQuery, []any{tenantID}},
		"device-command-bindings.jsonl": {deviceBindingsQuery, []any{tenantID}},
		"authority-events.jsonl":        {"SELECT * FROM device_authority_events ORDER BY event_id", nil},
		"safety-events.jsonl":           {"SELECT * FROM device_safety_events ORDER BY event_id", nil},
	}
}

type ledgerQuery struct {
	query string
	args  []any
}

const situationsQuery = `SELECT sv.* FROM situation_versions sv
			JOIN situations s ON s.situation_id = sv.situation_id
			WHERE s.tenant_id = ? ORDER BY sv.situation_id, sv.version`

const decisionsQuery = `SELECT d.* FROM decisions d
			JOIN episodes e ON e.episode_id = d.episode_id
			WHERE e.tenant_id = ? ORDER BY d.decision_id`

const deviceResultsQuery = `SELECT o.* FROM outcomes o
			JOIN commands c ON c.command_id = o.command_id
			WHERE c.tenant_id = ? ORDER BY o.command_id, o.ordinal`

const feedbackQuery = `SELECT v.* FROM verifications v
			JOIN commands c ON c.command_id = v.command_id
			WHERE c.tenant_id = ? ORDER BY v.verification_id`

const deviceBindingsQuery = `SELECT b.* FROM device_command_bindings b
			JOIN commands c ON c.command_id = b.command_id
			WHERE c.tenant_id = ? ORDER BY b.command_id`
