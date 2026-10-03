package runartifact

import (
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"fmt"

	"github.com/ghassan-ai-projects/agentic-stream/internal/canonicaljson"
)

func queryJSONL(ctx context.Context, tx *sql.Tx, query string, args ...any) ([]byte, error) {
	rows, err := tx.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query artifact rows: %w", err)
	}
	defer func() { _ = rows.Close() }()
	columns, err := rows.Columns()
	if err != nil {
		return nil, fmt.Errorf("read artifact columns: %w", err)
	}
	return collectArtifactRows(rows, columns)
}

func collectArtifactRows(rows *sql.Rows, columns []string) ([]byte, error) {
	var out []byte
	for rows.Next() {
		line, err := encodeArtifactRow(rows, columns)
		if err != nil {
			return nil, err
		}
		out = append(out, line...)
		out = append(out, '\n')
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate artifact rows: %w", err)
	}
	return out, nil
}

func encodeArtifactRow(rows *sql.Rows, columns []string) ([]byte, error) {
	values := make([]any, len(columns))
	pointers := make([]any, len(columns))
	for i := range values {
		pointers[i] = &values[i]
	}
	if err := rows.Scan(pointers...); err != nil {
		return nil, fmt.Errorf("scan artifact row: %w", err)
	}
	return canonicalArtifactRow(columns, values)
}

func canonicalArtifactRow(columns []string, values []any) ([]byte, error) {
	document := make(map[string]any, len(columns))
	for i, column := range columns {
		document[column] = databaseValue(values[i])
	}
	line, err := canonicaljson.Marshal(document)
	if err != nil {
		return nil, fmt.Errorf("canonicalize artifact row: %w", err)
	}
	return line, nil
}

func databaseValue(value any) any {
	data, ok := value.([]byte)
	if !ok {
		return value
	}
	if json.Valid(data) {
		var decoded any
		if json.Unmarshal(data, &decoded) == nil {
			return decoded
		}
	}
	return base64.StdEncoding.EncodeToString(data)
}
