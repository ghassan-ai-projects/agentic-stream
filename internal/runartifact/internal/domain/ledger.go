package domain

import (
	"encoding/base64"
	"encoding/json"
	"fmt"

	"github.com/ghassan-ai-projects/agentic-stream/internal/canonicaljson"
)

// LedgerTable is the raw result of one ledger query: column names and one value
// slice per row, as the database returned them.
type LedgerTable struct {
	Columns []string
	Rows    [][]any
}

// EncodeLedger renders a table as canonical JSON Lines. Binary values that are
// valid JSON are embedded as JSON; other binary values are base64.
func EncodeLedger(table LedgerTable) ([]byte, error) {
	var out []byte
	for _, values := range table.Rows {
		line, err := encodeLedgerRow(table.Columns, values)
		if err != nil {
			return nil, err
		}
		out = append(append(out, line...), '\n')
	}
	return out, nil
}

func encodeLedgerRow(columns []string, values []any) ([]byte, error) {
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
	var decoded any
	if json.Valid(data) && json.Unmarshal(data, &decoded) == nil {
		return decoded
	}
	return base64.StdEncoding.EncodeToString(data)
}
