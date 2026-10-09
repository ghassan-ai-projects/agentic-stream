package domain

import (
	"bytes"
	"crypto/sha256"
	"fmt"
)

// ReservationKey names one fenced evidence call.
type ReservationKey struct {
	TenantID  string
	EpisodeID string
	AttemptID string
	Fence     int64
	CallID    string
}

type Reservation struct {
	Key           ReservationKey
	RequestSHA256 []byte
	TokenID       string
	RuntimeEpoch  string
	Completed     *QueryResult
	Status        string
	Created       bool
}

// ReservationRow is the loaded durable call record.
type ReservationRow struct {
	RequestHash, ResultJSON, ResultHash []byte
	TokenID, Epoch, Status              string
	RowCount, ResultBytes               int64
}

// Reservation returns a verified exact result for a completed call.
func (row ReservationRow) Reservation(key ReservationKey, fingerprint []byte, tokenID, runtimeEpoch string) (*Reservation, error) {
	reservation := &Reservation{Key: key, RequestSHA256: fingerprint, TokenID: tokenID, RuntimeEpoch: runtimeEpoch, Status: row.Status}
	if row.Status != "completed" {
		return reservation, nil
	}
	if err := row.checkResultIntegrity(); err != nil {
		return nil, err
	}
	reservation.Completed = &QueryResult{JSON: append([]byte(nil), row.ResultJSON...), RowCount: uint64(row.RowCount)} //nolint:gosec // checkResultIntegrity rejects negative row counts.
	return reservation, nil
}
func (row ReservationRow) checkResultIntegrity() error {
	if row.ResultBytes < 0 || uint64(row.ResultBytes) != uint64(len(row.ResultJSON)) || len(row.ResultHash) != sha256.Size || row.RowCount < 0 {
		return fmt.Errorf("stored evidence result is malformed")
	}
	if !bytes.Equal(QueryResult{JSON: row.ResultJSON}.SHA256(), row.ResultHash) {
		return fmt.Errorf("stored evidence result digest mismatch")
	}
	return nil
}

// ExistingReservation refuses reuse before checking result integrity.
func ExistingReservation(row ReservationRow, key ReservationKey, fingerprint []byte, tokenID, epoch string) (*Reservation, error) {
	if string(row.RequestHash) != string(fingerprint) {
		return nil, fmt.Errorf("evidence call identity was reused with different request")
	}
	if row.TokenID != tokenID || row.Epoch != epoch {
		return nil, fmt.Errorf("evidence call token identity mismatch")
	}
	return row.Reservation(key, fingerprint, tokenID, epoch)
}
