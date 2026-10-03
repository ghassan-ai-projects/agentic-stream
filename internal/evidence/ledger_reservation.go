package evidence

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"errors"
	"fmt"
)

// loadReservation returns the existing reservation for key, or nil when the
// call is new. A reused call identity must carry the same request and token,
// and a completed call returns its verified stored result.
func loadReservation(ctx context.Context, tx *sql.Tx, key ledgerKey, fingerprint []byte, tokenID, runtimeEpoch string) (*ledgerReservation, error) {
	row, err := readReservation(ctx, tx, key)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if string(row.requestHash) != string(fingerprint) {
		return nil, fmt.Errorf("evidence call identity was reused with different request")
	}
	if row.tokenID != tokenID || row.epoch != runtimeEpoch {
		return nil, fmt.Errorf("evidence call token identity mismatch")
	}
	return row.reservation(key, fingerprint, tokenID, runtimeEpoch)
}

type reservationRow struct {
	requestHash, resultJSON, resultHash []byte
	tokenID, epoch, status              string
	rowCount, resultBytes               sql.NullInt64
}

func readReservation(ctx context.Context, tx *sql.Tx, key ledgerKey) (reservationRow, error) {
	var row reservationRow
	err := tx.QueryRowContext(ctx, `SELECT request_sha256, token_id, runtime_epoch, status, result_json, result_sha256, row_count, result_bytes FROM evidence_call_ledger WHERE tenant_id = ? AND episode_id = ? AND attempt_id = ? AND fence = ? AND call_id = ?`, key.TenantID, key.EpisodeID, key.AttemptID, key.Fence, key.CallID).Scan(&row.requestHash, &row.tokenID, &row.epoch, &row.status, &row.resultJSON, &row.resultHash, &row.rowCount, &row.resultBytes)
	if err != nil {
		return row, fmt.Errorf("load evidence call: %w", err)
	}
	return row, nil
}

func (row reservationRow) reservation(key ledgerKey, fingerprint []byte, tokenID, runtimeEpoch string) (*ledgerReservation, error) {
	reservation := &ledgerReservation{Key: key, RequestSHA256: fingerprint, TokenID: tokenID, RuntimeEpoch: runtimeEpoch, Status: row.status, ResultSHA256: row.resultHash}
	if row.status != "completed" {
		return reservation, nil
	}
	if err := row.checkResultIntegrity(); err != nil {
		return nil, err
	}
	reservation.Completed = &QueryResult{JSON: append([]byte(nil), row.resultJSON...), RowCount: uint64(row.rowCount.Int64)} //nolint:gosec // checkResultIntegrity rejects negative row counts.
	return reservation, nil
}

func (row reservationRow) checkResultIntegrity() error {
	if row.resultBytes.Int64 < 0 || uint64(row.resultBytes.Int64) != uint64(len(row.resultJSON)) || len(row.resultHash) != sha256.Size || row.rowCount.Int64 < 0 {
		return fmt.Errorf("stored evidence result is malformed")
	}
	hash := sha256.Sum256(row.resultJSON)
	if string(hash[:]) != string(row.resultHash) {
		return fmt.Errorf("stored evidence result digest mismatch")
	}
	return nil
}
