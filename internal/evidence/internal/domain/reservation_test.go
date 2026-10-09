package domain

import (
	"crypto/sha256"
	"testing"
)

var reservationKey = ReservationKey{TenantID: "tenant-1", EpisodeID: "episode-1", AttemptID: "attempt-1", Fence: 1, CallID: "call-1"}

func completedRow() ReservationRow {
	result := []byte(`[]`)
	hash := sha256.Sum256(result)
	return ReservationRow{RequestHash: []byte("request"), TokenID: "token", Epoch: "epoch", Status: "completed", ResultJSON: result, ResultHash: hash[:], ResultBytes: 2, RowCount: 1}
}

func TestExistingReservationReturnsAVerifiedCompletedResult(t *testing.T) {
	t.Parallel()
	row := completedRow()
	got, err := ExistingReservation(row, reservationKey, row.RequestHash, "token", "epoch")
	if err != nil || got.Completed == nil || string(got.Completed.JSON) != `[]` || got.Completed.RowCount != 1 || got.Status != "completed" {
		t.Fatalf("reservation = %+v, err %v", got, err)
	}
	got.Completed.JSON[0] = 'x'
	if row.ResultJSON[0] != '[' {
		t.Fatal("the returned result aliases the stored bytes")
	}
}

func TestExistingReservationOfAnUnfinishedCallCarriesNoResult(t *testing.T) {
	t.Parallel()
	for _, status := range []string{"running", "failed", "interrupted"} {
		t.Run(status, func(t *testing.T) {
			t.Parallel()
			row := ReservationRow{RequestHash: []byte("request"), TokenID: "token", Epoch: "epoch", Status: status}
			got, err := ExistingReservation(row, reservationKey, row.RequestHash, "token", "epoch")
			if err != nil || got.Completed != nil || got.Status != status {
				t.Fatalf("reservation = %+v, err %v", got, err)
			}
		})
	}
}

func TestExistingReservationRefusesReuseAndCorruption(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		mutate func(*ReservationRow)
		want   string
	}{
		{"different request", func(r *ReservationRow) { r.RequestHash = []byte("other") }, "different request"},
		{"missing request", func(r *ReservationRow) { r.RequestHash = nil }, "different request"},
		{"different token", func(r *ReservationRow) { r.TokenID = "other" }, "token identity mismatch"},
		{"different runtime epoch", func(r *ReservationRow) { r.Epoch = "other" }, "token identity mismatch"},
		{"negative size", func(r *ReservationRow) { r.ResultBytes = -1 }, "malformed"},
		{"size not matching the bytes", func(r *ReservationRow) { r.ResultBytes = 3 }, "malformed"},
		{"negative row count", func(r *ReservationRow) { r.RowCount = -1 }, "malformed"},
		{"truncated digest", func(r *ReservationRow) { r.ResultHash = r.ResultHash[:8] }, "malformed"},
		{"digest of other bytes", func(r *ReservationRow) { r.ResultHash = make([]byte, sha256.Size) }, "digest mismatch"},
		{"identity reuse before integrity", func(r *ReservationRow) {
			r.RequestHash = []byte("other")
			r.ResultBytes = -1
		}, "different request"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			row := completedRow()
			request := row.RequestHash
			test.mutate(&row)
			_, err := ExistingReservation(row, reservationKey, request, "token", "epoch")
			requireErrorContaining(t, err, test.want)
		})
	}
}

func TestReservationRefusalNamesWhyACallCannotRunAgain(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name        string
		reservation Reservation
		want        ErrorKind
	}{
		{"created now", Reservation{Created: true}, ""},
		{"failed earlier", Reservation{Status: "failed"}, FailedPrecondition},
		{"interrupted earlier", Reservation{Status: "interrupted"}, FailedPrecondition},
		{"still running", Reservation{Status: "running"}, AlreadyExists},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			err := ReservationRefusal(test.reservation)
			if test.want == "" {
				if err != nil {
					t.Fatalf("ReservationRefusal: %v", err)
				}
				return
			}
			requireRefusal(t, err, test.want)
		})
	}
}

func TestResultViolationNamesTheExceededBudget(t *testing.T) {
	t.Parallel()
	call := Call{MaxRows: 2, MaxBytes: 3}
	tests := []struct {
		name     string
		result   QueryResult
		wantCode string
	}{
		{"within both budgets", QueryResult{JSON: []byte(`123`), RowCount: 2}, ""},
		{"empty result", QueryResult{}, ""},
		{"one byte too many", QueryResult{JSON: []byte(`1234`), RowCount: 1}, "result_bytes_exceeded"},
		{"one row too many", QueryResult{JSON: []byte(`1`), RowCount: 3}, "result_rows_exceeded"},
		{"bytes are reported before rows", QueryResult{JSON: []byte(`1234`), RowCount: 3}, "result_bytes_exceeded"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			code, err := ResultViolation(call, test.result)
			if code != test.wantCode {
				t.Fatalf("code = %q, want %q", code, test.wantCode)
			}
			if test.wantCode == "" {
				if err != nil {
					t.Fatalf("ResultViolation: %v", err)
				}
				return
			}
			requireRefusal(t, err, ResourceExhausted)
		})
	}
}

func TestQueryResultDigestCoversExactBytes(t *testing.T) {
	t.Parallel()
	result := QueryResult{JSON: []byte(`{"rows":[]}`)}
	want := sha256.Sum256(result.JSON)
	if got := result.SHA256(); string(got) != string(want[:]) {
		t.Fatalf("digest = %x, want %x", got, want)
	}
}
