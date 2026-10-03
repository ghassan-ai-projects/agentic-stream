package eventlog

import (
	"cmp"
	"context"
	"database/sql"
	"fmt"
)

// ReadRequest selects a range of records from the log.
type ReadRequest struct {
	TenantID      string
	PartitionID   int
	AfterPosition LogPosition
	Limit         int
}

// Read streams records matching req into callback.
func (l *EventLog) Read(ctx context.Context, req ReadRequest, callback func(Record) error) error {
	rows, err := l.queryRecords(ctx, req)
	if err != nil {
		return err
	}
	defer func() { _ = rows.Close() }()

	return deliverRecords(rows, callback)
}

func (l *EventLog) queryRecords(ctx context.Context, req ReadRequest) (*sql.Rows, error) {
	query, args := recordQuery(req)
	rows, err := l.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query event log: %w", err)
	}
	return rows, nil
}

// recordQuery selects the tenant's records after a position, optionally in
// one partition, in log order; the limit defaults to 1000.
func recordQuery(req ReadRequest) (string, []any) {
	query := selectRecordsSQL
	args := []any{req.TenantID, req.AfterPosition}
	if req.PartitionID >= 0 {
		query += " AND partition_id = ?"
		args = append(args, req.PartitionID)
	}
	return query + " ORDER BY position LIMIT ?", append(args, cmp.Or(max(req.Limit, 0), 1000))
}

const selectRecordsSQL = `
		SELECT position, tenant_id, partition_id, event_id, event_type,
		       schema_version, source, partition_key, entity_type, entity_id,
		       event_time, observed_at, ingested_at, correlation_id,
		       causation_id, traceparent, tracestate, classification, quality_json,
		       payload_json
		FROM event_log
		WHERE tenant_id = ? AND position > ?`

func deliverRecords(rows *sql.Rows, callback func(Record) error) error {
	for rows.Next() {
		rec, err := scanRecord(rows)
		if err != nil {
			return err
		}
		if err := callback(rec); err != nil {
			return err
		}
	}

	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate event log: %w", err)
	}
	return nil
}
