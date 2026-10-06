package app

import (
	"context"
	"github.com/ghassan-ai-projects/agentic-stream/internal/evidence/internal/domain"
	"time"
)

// Server sequences admitted durable evidence queries.
type Server struct {
	Verifier     *Verifier
	Query        Query
	Now          func() time.Time
	Ledger       *Ledger
	RuntimeEpoch string
}

// Call authorizes, reserves, executes and records one bounded evidence query.
func (s *Server) Call(ctx context.Context, req domain.Envelope) (QueryResult, error) {
	now := s.now()
	call, scope, err := s.admitCall(req, now)
	if err != nil {
		return QueryResult{}, err
	}
	reservation, err := s.reserve(ctx, call, scope)
	if err != nil {
		return QueryResult{}, err
	}
	if reservation.Completed != nil {
		return *reservation.Completed, nil
	}
	return s.queryResult(ctx, call, reservation, now)
}
func (s *Server) reserve(ctx context.Context, call Call, scope Scope) (ledgerReservation, error) {
	reservation, err := s.Ledger.Reserve(ctx, call, scope.TokenID, s.RuntimeEpoch)
	if err != nil {
		return ledgerReservation{}, domain.Refuse(domain.FailedPrecondition, "evidence call reservation failed")
	}
	if reservation.Completed != nil {
		return reservation, nil
	}
	if err := domain.ReservationRefusal(reservation); err != nil {
		return ledgerReservation{}, err
	}
	return reservation, nil
}

func (s *Server) commitResult(ctx context.Context, reservation ledgerReservation, result QueryResult) error {
	if err := s.Ledger.Complete(ctx, reservation, result); err != nil {
		return domain.Refuse(domain.FailedPrecondition, "evidence result could not be committed")
	}
	return nil
}
func (s *Server) now() time.Time {
	if s.Now != nil {
		return s.Now().UTC()
	}
	return time.Now().UTC()
}
func (s *Server) runQuery(ctx context.Context, call Call, reservation ledgerReservation, now time.Time) (QueryResult, error) {
	queryContext, cancel := context.WithTimeout(ctx, call.Deadline.Sub(now))
	defer cancel()
	result, err := s.Query(queryContext, call)
	if err != nil {
		return QueryResult{}, s.queryFailure(ctx, queryContext, call, reservation)
	}
	if queryContext.Err() != nil {
		return QueryResult{}, domain.ContextRefusal(queryContext.Err())
	}
	if err := s.checkResultBounds(ctx, call, reservation, result); err != nil {
		return QueryResult{}, err
	}
	return result, nil
}
func (s *Server) queryFailure(ctx, queryContext context.Context, call Call, reservation ledgerReservation) error {
	s.failCall(ctx, reservation, "query_failed")
	if queryContext.Err() != nil {
		return domain.ContextRefusal(queryContext.Err())
	}
	return domain.Refuse(domain.Internal, "evidence query failed") // Do not leak query details.
}
func (s *Server) checkResultBounds(ctx context.Context, call Call, reservation ledgerReservation, result QueryResult) error {
	code, err := domain.ResultViolation(call, result)
	if err != nil {
		s.failCall(ctx, reservation, code)
	}
	return err
}
func (s *Server) failCall(ctx context.Context, reservation ledgerReservation, code string) {
	_ = s.Ledger.Fail(ctx, reservation, code)
}
func (s *Server) queryResult(ctx context.Context, call Call, reservation ledgerReservation, now time.Time) (QueryResult, error) {
	result, err := s.runQuery(ctx, call, reservation, now)
	if err != nil {
		return QueryResult{}, err
	}
	if err := s.commitResult(ctx, reservation, result); err != nil {
		return QueryResult{}, err
	}
	return result, nil
}
