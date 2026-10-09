package app

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/evidence/internal/domain"
)

func TestCallReturnsTheBoundedResultAndReplaysItWithoutQuerying(t *testing.T) {
	t.Parallel()
	fixture := newCallFixture(t, callOptions{})
	first, err := fixture.service.Call(t.Context(), workerEnvelope(fixture.token, "call-1"))
	if err != nil {
		t.Fatalf("first call: %v", err)
	}
	second, err := fixture.service.Call(t.Context(), workerEnvelope(fixture.token, "call-1"))
	if err != nil {
		t.Fatalf("replayed call: %v", err)
	}
	if *fixture.queries != 1 {
		t.Fatalf("provider queries = %d, want 1: a completed call replays its stored result", *fixture.queries)
	}
	if string(first.JSON) != `{"rows":[{"value":42}]}` || string(second.JSON) != string(first.JSON) || second.RowCount != 1 {
		t.Fatalf("results differ: %+v vs %+v", first, second)
	}
	if hash := sha256.Sum256(second.JSON); string(second.SHA256()) != string(hash[:]) {
		t.Fatal("the replayed result does not carry the digest of its bytes")
	}
	if status, _ := ledgerStatus(t, fixture.db, "call-1"); status != "completed" {
		t.Fatalf("ledger status = %q, want completed", status)
	}
}

func TestCallEnforcesResultBoundsAndRecordsStableFailureCodes(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		result   QueryResult
		queryErr error
		want     domain.ErrorKind
		wantCode string
	}{
		{name: "too many bytes", result: QueryResult{JSON: make([]byte, 101)}, want: domain.ResourceExhausted, wantCode: "result_bytes_exceeded"},
		{name: "too many rows", result: QueryResult{JSON: []byte(`[]`), RowCount: 2}, want: domain.ResourceExhausted, wantCode: "result_rows_exceeded"},
		{name: "provider failure", queryErr: errors.New("database is gone"), want: domain.Internal, wantCode: "query_failed"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			fixture := newCallFixture(t, callOptions{query: func(context.Context, Call) (QueryResult, error) { return test.result, test.queryErr }})

			_, err := fixture.service.Call(t.Context(), workerEnvelope(fixture.token, "call-1"))
			refusal := refusalFrom(t, err, test.want)
			if test.queryErr != nil && refusal.Message != "evidence query failed" {
				t.Fatalf("refusal message %q exposes provider details", refusal.Message)
			}
			if status, code := ledgerStatus(t, fixture.db, "call-1"); status != "failed" || code != test.wantCode {
				t.Fatalf("ledger = %q/%q, want failed/%s", status, code, test.wantCode)
			}
			_, err = fixture.service.Call(t.Context(), workerEnvelope(fixture.token, "call-1"))
			requireRefusal(t, err, domain.FailedPrecondition)
			if *fixture.queries != 1 {
				t.Fatalf("provider queries = %d: a failed call identity must never query again", *fixture.queries)
			}
		})
	}
}

func TestCallAcceptsAResultExactlyAtTheBudgets(t *testing.T) {
	t.Parallel()
	fixture := newCallFixture(t, callOptions{query: func(context.Context, Call) (QueryResult, error) {
		return QueryResult{JSON: make([]byte, 100), RowCount: 1}, nil
	}})
	if _, err := fixture.service.Call(t.Context(), workerEnvelope(fixture.token, "call-1")); err != nil {
		t.Fatalf("Call: %v", err)
	}
}

func TestCanceledCallRecordsItsFailureOnADetachedContext(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	fixture := newCallFixture(t, callOptions{query: func(context.Context, Call) (QueryResult, error) {
		cancel()
		return QueryResult{}, errors.New("provider private details")
	}})
	_, err := fixture.service.Call(ctx, workerEnvelope(fixture.token, "call-1"))
	requireRefusal(t, err, domain.Canceled)
	if status, code := ledgerStatus(t, fixture.db, "call-1"); status != "failed" || code != "query_failed" {
		t.Fatalf("ledger = %q/%q, want failed/query_failed", status, code)
	}
}

func TestCallDeadlineBoundsTheProviderQuery(t *testing.T) {
	t.Parallel()
	fixture := newCallFixture(t, callOptions{query: func(ctx context.Context, _ Call) (QueryResult, error) {
		<-ctx.Done()
		return QueryResult{}, fmt.Errorf("provider stopped: %w", ctx.Err())
	}})
	request := workerEnvelope(fixture.token, "call-1")
	request.Deadline.Value = testNow.Add(20 * time.Millisecond)
	_, err := fixture.service.Call(t.Context(), request)
	requireRefusal(t, err, domain.DeadlineExceeded)
	if status, code := ledgerStatus(t, fixture.db, "call-1"); status != "failed" || code != "query_failed" {
		t.Fatalf("ledger = %q/%q, want failed/query_failed", status, code)
	}
}

func TestCallDiscardsAResultThatArrivesAfterItsDeadline(t *testing.T) {
	t.Parallel()
	fixture := newCallFixture(t, callOptions{query: func(ctx context.Context, _ Call) (QueryResult, error) {
		<-ctx.Done()
		return QueryResult{JSON: []byte(`{"rows":[]}`)}, nil
	}})
	request := workerEnvelope(fixture.token, "call-1")
	request.Deadline.Value = testNow.Add(20 * time.Millisecond)
	result, err := fixture.service.Call(t.Context(), request)
	requireRefusal(t, err, domain.DeadlineExceeded)
	if result.JSON != nil {
		t.Fatalf("a late result was returned: %s", result.JSON)
	}
	if status, _ := ledgerStatus(t, fixture.db, "call-1"); status == "completed" {
		t.Fatal("a late result was stored as completed")
	}
}

func TestCallRefusesADuplicateWhileTheFirstIsStillRunning(t *testing.T) {
	t.Parallel()
	started, release := make(chan struct{}), make(chan struct{})
	fixture := newCallFixture(t, callOptions{query: func(context.Context, Call) (QueryResult, error) {
		close(started)
		<-release
		return QueryResult{JSON: []byte(`{"rows":[]}`)}, nil
	}})
	first := make(chan error, 1)
	go func() {
		_, err := fixture.service.Call(t.Context(), workerEnvelope(fixture.token, "call-1"))
		first <- err
	}()
	<-started

	_, err := fixture.service.Call(t.Context(), workerEnvelope(fixture.token, "call-1"))
	requireRefusal(t, err, domain.AlreadyExists)
	close(release)
	if err := <-first; err != nil {
		t.Fatalf("first call: %v", err)
	}
	if status, _ := ledgerStatus(t, fixture.db, "call-1"); status != "completed" {
		t.Fatalf("ledger status = %q, want completed", status)
	}
}

func TestCallRefusesAReusedCallIdentityWithADifferentRequest(t *testing.T) {
	t.Parallel()
	fixture := newCallFixture(t, callOptions{})
	if _, err := fixture.service.Call(t.Context(), workerEnvelope(fixture.token, "call-1")); err != nil {
		t.Fatalf("first call: %v", err)
	}
	changed := workerEnvelope(fixture.token, "call-1")
	changed.Until.Value = testNow
	_, err := fixture.service.Call(t.Context(), changed)
	requireRefusal(t, err, domain.FailedPrecondition)
	if *fixture.queries != 1 {
		t.Fatalf("provider queries = %d, want 1", *fixture.queries)
	}
}

func TestCallRefusesStaleEpisodesBeforeQuerying(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		statement string
	}{
		{"superseded episode", "UPDATE episodes SET lifecycle_status = 'superseded'"},
		{"concluded episode", "UPDATE episodes SET lifecycle_status = 'concluded'"},
		{"newer fence on the episode", "UPDATE episodes SET current_fence = 2"},
		{"terminal attempt", "UPDATE episode_attempts SET status = 'cancelled'"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			fixture := newCallFixture(t, callOptions{})
			execSQL(t, fixture.db, test.statement)
			_, err := fixture.service.Call(t.Context(), workerEnvelope(fixture.token, "call-1"))
			requireRefusal(t, err, domain.FailedPrecondition)
			if *fixture.queries != 0 || ledgerRows(t, fixture.db) != 0 {
				t.Fatalf("a stale attempt reached the provider (%d) or reserved (%d rows)", *fixture.queries, ledgerRows(t, fixture.db))
			}
		})
	}
}

func TestCallCannotCommitAResultOnceTheAttemptIsSuperseded(t *testing.T) {
	t.Parallel()
	var fixture *callFixture
	fixture = newCallFixture(t, callOptions{query: func(context.Context, Call) (QueryResult, error) {
		execSQL(t, fixture.db, "UPDATE episodes SET lifecycle_status = 'superseded'")
		return QueryResult{JSON: []byte(`{"rows":[]}`)}, nil
	}})
	result, err := fixture.service.Call(t.Context(), workerEnvelope(fixture.token, "call-1"))
	refusal := refusalFrom(t, err, domain.FailedPrecondition)
	if refusal.Message != "evidence result could not be committed" || result.JSON != nil {
		t.Fatalf("refusal %q with result %s, want an uncommitted result withheld from the worker", refusal.Message, result.JSON)
	}
	if status, _ := ledgerStatus(t, fixture.db, "call-1"); status != "running" {
		t.Fatalf("ledger status = %q, want running", status)
	}
}

func TestCallRefusesWhenTheRuntimeOwnerIsLost(t *testing.T) {
	t.Parallel()
	lost := errors.New("owner lost")
	fixture := newCallFixture(t, callOptions{owner: func(context.Context, *sql.Tx, string) error { return lost }})
	_, err := fixture.service.Call(t.Context(), workerEnvelope(fixture.token, "call-1"))
	requireRefusal(t, err, domain.FailedPrecondition)
	if *fixture.queries != 0 || ledgerRows(t, fixture.db) != 0 {
		t.Fatalf("a call without ownership queried (%d) or reserved (%d rows)", *fixture.queries, ledgerRows(t, fixture.db))
	}
}
