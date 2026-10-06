package domain

import (
	"context"
	"crypto/sha256"
	"errors"
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/contractsv1"
)

func validEnvelope(s Scope) Envelope {
	return Envelope{ProtocolVersion: "1.0", EpisodeID: s.EpisodeID, CallID: "call", ToolName: s.Tools[0], TenantID: s.TenantID, SituationID: s.SituationID, EntityID: s.EntityID, AttemptID: s.AttemptID, Fence: 1, SituationVersion: 1, MaxRows: 1, MaxBytes: 100, ArgumentsJSON: []byte(`{"entity_id":"x"}`), Traceparent: s.Traceparent, From: Timestamp{Value: s.From, Present: true, Valid: true}, Until: Timestamp{Value: s.Until, Present: true, Valid: true}}
}
func refusalKind(t *testing.T, err error, want ErrorKind) {
	t.Helper()
	var got *Refusal
	if !errors.As(err, &got) || got.Kind != want {
		t.Fatalf("error=%v want=%s", err, want)
	}
}

func TestAdmissionRulesPreserveOrderAndBounds(t *testing.T) {
	s := completeScope()
	req := validEnvelope(s)
	if err := ValidateEnvelope(req); err != nil {
		t.Fatal(err)
	}
	bad := req
	bad.EpisodeID = ""
	bad.ArgumentsJSON = make([]byte, MaxArgumentsBytes+1)
	refusalKind(t, ValidateEnvelope(bad), InvalidArgument)
	bad = req
	bad.EncodedSize = MaxArgumentsBytes + MaxCapabilityTokenBytes + 1
	refusalKind(t, ValidateEnvelope(bad), ResourceExhausted)
	for name, mutate := range map[string]func(*Envelope){"fence": func(e *Envelope) { e.Fence = 1 << 63 }, "version": func(e *Envelope) { e.SituationVersion = 1 << 63 }} {
		t.Run(name, func(t *testing.T) {
			e := req
			mutate(&e)
			refusalKind(t, AuthorizeScope(e, s, "epoch"), InvalidArgument)
		})
	}
	bad = req
	bad.AttemptID = "other"
	bad.From = Timestamp{}
	refusalKind(t, AuthorizeScope(bad, s, "epoch"), PermissionDenied)
	bad = req
	bad.Until.Value = s.Until.Add(time.Second)
	refusalKind(t, AuthorizeScope(bad, s, "epoch"), PermissionDenied)
	if err := AuthorizeScope(req, s, "epoch"); err != nil {
		t.Fatal(err)
	}
	call, err := BindArguments(req, s, contractsv1.TraceContext{Traceparent: s.Traceparent}, EvidenceGetArguments{EntityID: s.EntityID})
	if err != nil || call.MaxRows != 1 || call.MaxBytes != 100 || call.Fence != s.Fence {
		t.Fatalf("call=%+v err=%v", call, err)
	}
	_, err = BindArguments(req, s, contractsv1.TraceContext{}, EvidenceGetArguments{EntityID: "other"})
	refusalKind(t, err, PermissionDenied)
	if got := minNonZero(0, 5); got != 5 {
		t.Fatal(got)
	}
	if got := minNonZero(10, 5); got != 5 {
		t.Fatal(got)
	}
	now := s.IssuedAt
	deadline, err := CallDeadline(req, s, now)
	if err != nil || !deadline.Equal(s.ExpiresAt) {
		t.Fatalf("deadline=%v err=%v", deadline, err)
	}
	bad = req
	bad.Deadline = Timestamp{Present: true, Valid: false}
	_, err = CallDeadline(bad, s, now)
	refusalKind(t, err, InvalidArgument)
	bad.Deadline = Timestamp{Present: true, Valid: true, Value: now}
	_, err = CallDeadline(bad, s, now)
	refusalKind(t, err, DeadlineExceeded)
	bad.Deadline.Value = now.Add(time.Second)
	deadline, err = CallDeadline(bad, s, now)
	if err != nil || !deadline.Equal(bad.Deadline.Value) {
		t.Fatal(deadline, err)
	}
}

func TestAttemptAndReservationRules(t *testing.T) {
	call := Call{AttemptID: "a", Fence: 1}
	key := ReservationKey{AttemptID: "a", Fence: 1}
	state := EpisodeState{Lifecycle: "running", AttemptID: "a", Fence: 1}
	if err := CheckLiveEpisode(state, call); err != nil {
		t.Fatal(err)
	}
	if err := CheckCompletionEpisode(state, key); err != nil {
		t.Fatal(err)
	}
	for _, status := range []string{"concluded", "closed", "superseded", "expired", "abandoned"} {
		state.Lifecycle = status
		if CheckLiveEpisode(state, call) == nil || CheckCompletionEpisode(state, key) == nil {
			t.Fatalf("terminal=%s", status)
		}
	}
	for _, status := range []string{"dispatched", "running"} {
		if CheckLiveAttempt(status) != nil || CheckCompletionAttempt(status) != nil {
			t.Fatal(status)
		}
	}
	if CheckLiveAttempt("failed") == nil || CheckCompletionAttempt("failed") == nil {
		t.Fatal("terminal attempt accepted")
	}
	if err := ReservationRefusal(Reservation{Created: true}); err != nil {
		t.Fatal(err)
	}
	refusalKind(t, ReservationRefusal(Reservation{Status: "failed"}), FailedPrecondition)
	refusalKind(t, ReservationRefusal(Reservation{Status: "running"}), AlreadyExists)
	result := QueryResult{JSON: []byte(`[]`), RowCount: 1}
	hash := sha256.Sum256(result.JSON)
	row := ReservationRow{RequestHash: []byte("request"), TokenID: "token", Epoch: "epoch", Status: "completed", ResultJSON: result.JSON, ResultHash: hash[:], ResultBytes: 2, RowCount: 1}
	got, err := ExistingReservation(row, key, row.RequestHash, "token", "epoch")
	if err != nil || got.Completed == nil {
		t.Fatalf("reservation=%+v err=%v", got, err)
	}
	got.Completed.JSON[0] = 'x'
	if row.ResultJSON[0] != '[' {
		t.Fatal("stored bytes aliased")
	}
	for name, mutate := range map[string]func(*ReservationRow){"request": func(r *ReservationRow) { r.RequestHash = nil }, "token": func(r *ReservationRow) { r.TokenID = "other" }, "size": func(r *ReservationRow) { r.ResultBytes = -1 }, "rows": func(r *ReservationRow) { r.RowCount = -1 }, "hash": func(r *ReservationRow) { r.ResultHash = make([]byte, 32) }} {
		t.Run(name, func(t *testing.T) {
			r := row
			mutate(&r)
			if _, err := ExistingReservation(r, key, row.RequestHash, "token", "epoch"); err == nil {
				t.Fatal("invalid reservation accepted")
			}
		})
	}
	row.Status = "running"
	got, err = ExistingReservation(row, key, row.RequestHash, "token", "epoch")
	if err != nil || got.Completed != nil {
		t.Fatal(got, err)
	}
}
func TestQueryFailureCategories(t *testing.T) {
	refusalKind(t, ContextRefusal(context.Canceled), Canceled)
	refusalKind(t, ContextRefusal(context.DeadlineExceeded), DeadlineExceeded)
	call := Call{MaxRows: 1, MaxBytes: 2}
	code, err := ResultViolation(call, QueryResult{JSON: []byte(`123`), RowCount: 2})
	if code != "result_bytes_exceeded" {
		t.Fatal(code)
	}
	refusalKind(t, err, ResourceExhausted)
	code, err = ResultViolation(call, QueryResult{RowCount: 2})
	if code != "result_rows_exceeded" {
		t.Fatal(code)
	}
	refusalKind(t, err, ResourceExhausted)
	if code, err = ResultViolation(call, QueryResult{}); err != nil || code != "" {
		t.Fatal(code, err)
	}
}
