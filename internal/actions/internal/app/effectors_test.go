package app_test

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/actionport"
	"github.com/ghassan-ai-projects/agentic-stream/internal/actions/internal/app"
	"github.com/ghassan-ai-projects/agentic-stream/internal/actions/internal/store"
	"github.com/ghassan-ai-projects/agentic-stream/internal/interlock"
	"github.com/ghassan-ai-projects/agentic-stream/internal/sources"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
)

var acceptedEffect = actionport.Effect{ProviderResult: map[string]any{"accepted": true}}

var unknownOutcome = &actionport.UnknownOutcomeError{Err: errors.New("provider timeout")}

type scriptedEffector struct {
	calls    int
	effect   actionport.Effect
	err      error
	during   func()
	deadline time.Time
}

func succeeds() *scriptedEffector { return &scriptedEffector{effect: acceptedEffect} }

func failsWith(err error) *scriptedEffector { return &scriptedEffector{err: err} }

func pendingVerification() *scriptedEffector {
	return &scriptedEffector{effect: actionport.Effect{ProviderResult: acceptedEffect.ProviderResult, VerificationPending: true}}
}

func (e *scriptedEffector) Dispatch(ctx context.Context, _ actionport.Command) (actionport.Effect, error) {
	e.calls++
	e.deadline, _ = ctx.Deadline()
	if e.during != nil {
		e.during()
	}
	return e.effect, e.err
}

func (e *scriptedEffector) DispatchAuthorized(ctx context.Context, command actionport.Command, authorization actionport.Authorization) (actionport.Effect, error) {
	if err := authorization.Check(ctx); err != nil {
		return actionport.Effect{}, fmt.Errorf("check authorization: %w", err)
	}
	return e.Dispatch(ctx, command)
}

type deviceEffector struct {
	scriptedEffector
	verifyCalls    int
	finalStatus    string
	evidence       map[string]any
	verifyErr      error
	verifyDeadline time.Time
}

func (e *deviceEffector) VerifyDeviceCommand(ctx context.Context, _ actionport.Command) (string, map[string]any, error) {
	e.verifyCalls++
	e.verifyDeadline, _ = ctx.Deadline()
	return e.finalStatus, e.evidence, e.verifyErr
}

type tripBeforeAcceptEffector struct {
	db    *storage.DB
	calls int
}

func (e *tripBeforeAcceptEffector) Dispatch(context.Context, actionport.Command) (actionport.Effect, error) {
	return actionport.Effect{}, errors.New("unauthorized dispatch path")
}

func (e *tripBeforeAcceptEffector) DispatchAuthorized(ctx context.Context, _ actionport.Command, authorization actionport.Authorization) (actionport.Effect, error) {
	if err := e.db.WithTx(ctx, func(tx *sql.Tx) error {
		_, err := interlock.TripIn(ctx, tx, "race stop", fixtureNow)
		return err
	}); err != nil {
		return actionport.Effect{}, fmt.Errorf("trip interlock: %w", err)
	}
	if err := authorization.Check(ctx); err != nil {
		return actionport.Effect{}, fmt.Errorf("check authorization: %w", err)
	}
	e.calls++
	return acceptedEffect, nil
}

type dispatcherSpec struct {
	clock    sources.Clock
	owner    storage.OwnerCheck
	name     string
	leaseFor time.Duration
	observer app.LeaseObserver
}

type dispatcherOption func(*dispatcherSpec)

func withClock(clock sources.Clock) dispatcherOption {
	return func(s *dispatcherSpec) { s.clock = clock }
}

func withOwnerCheck(owner storage.OwnerCheck) dispatcherOption {
	return func(s *dispatcherSpec) { s.owner = owner }
}

func withName(name string) dispatcherOption { return func(s *dispatcherSpec) { s.name = name } }

func withLease(leaseFor time.Duration) dispatcherOption {
	return func(s *dispatcherSpec) { s.leaseFor = leaseFor }
}

func withObserver(observer app.LeaseObserver) dispatcherOption {
	return func(s *dispatcherSpec) { s.observer = observer }
}

func newDispatcher(t *testing.T, db *storage.DB, effector actionport.AuthorizedEffector, options ...dispatcherOption) *app.Service {
	t.Helper()
	spec := dispatcherSpec{clock: sources.NewVirtual(fixtureNow), owner: allowOwner, name: "dispatcher", leaseFor: time.Minute}
	for _, option := range options {
		option(&spec)
	}
	service, err := app.New(app.Config{Store: store.New(db, spec.owner, "epoch"), Effector: effector, Clock: spec.clock,
		IDs: sources.Deterministic(), LeaseOwner: spec.name, LeaseFor: spec.leaseFor, Observer: spec.observer})
	if err != nil {
		t.Fatalf("new action service: %v", err)
	}
	return service
}

func allowOwner(context.Context, *sql.Tx, string) error { return nil }

func dispatchOnce(t *testing.T, dispatcher *app.Service) bool {
	t.Helper()
	processed, err := dispatcher.DispatchOnce(t.Context())
	if err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	return processed
}
