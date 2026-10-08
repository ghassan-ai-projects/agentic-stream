package watch

import (
	"context"
	"database/sql"

	"github.com/ghassan-ai-projects/agentic-stream/internal/actionport"
	"github.com/ghassan-ai-projects/agentic-stream/internal/sources"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
	"github.com/ghassan-ai-projects/agentic-stream/internal/watch/internal/app"
	"github.com/ghassan-ai-projects/agentic-stream/internal/watch/internal/store"
)

// Config supplies the safety dependencies a watch service requires. DB
// and RuntimeOwner are required: a missing one is a constructor
// error, never a silently skipped check. RuntimeOwner is the ownership
// assertion for Epoch, run on the mutating transaction;
// control.RuntimeOwner.Assert is the production check. Clock defaults to the
// physical clock.
type Config struct {
	DB           *storage.DB
	RuntimeOwner func(context.Context, *sql.Tx, string) error
	Epoch        string
	Clock        sources.Clock
}

// Service installs, fires and expires watch conditions. It implements the
// effect port for the install route.
type Service struct{ app *app.Service }

// New validates the configuration and composes the watch use cases.
func New(cfg Config) (*Service, error) {
	service, err := app.New(app.Config{Store: store.New(cfg.DB, cfg.RuntimeOwner, cfg.Epoch), Clock: cfg.Clock})
	if err != nil {
		return nil, err //nolint:wrapcheck // The app layer's constructor errors are the facade's contract.
	}
	return &Service{app: service}, nil
}

// Dispatch installs one watch condition and is idempotent by watch identity.
func (s *Service) Dispatch(ctx context.Context, command actionport.Command) (actionport.Effect, error) {
	return s.app.Dispatch(ctx, command)
}

// DispatchAuthorized performs the final authorization check before install.
func (s *Service) DispatchAuthorized(ctx context.Context, command actionport.Command, authorization actionport.Authorization) (actionport.Effect, error) {
	return s.app.DispatchAuthorized(ctx, command, authorization)
}

// Expire marks due active watches expired without deleting their audit rows.
func (s *Service) Expire(ctx context.Context) error { return s.app.Expire(ctx) }

// FireEvent fires every active watch scoped to the event's target whose
// expression matches, and reports how many fired.
func (s *Service) FireEvent(ctx context.Context, eventID, target string, features map[string]any) (int, error) {
	return s.app.FireEvent(ctx, eventID, target, features)
}
