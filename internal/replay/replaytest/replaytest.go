// Package replaytest is test support for the replay module: it makes replays
// started with a context open their isolated database through a caller-chosen
// opener, for example one that copies a migrated template instead of running
// the migrations. Production always opens a fresh database with
// storage.OpenFresh. Import it from tests only.
package replaytest

import (
	"context"

	"github.com/ghassan-ai-projects/agentic-stream/internal/replay/internal/transport"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
)

// WithDatabaseOpener returns a context under which every replay opens its
// isolated database with open instead of storage.OpenFresh.
func WithDatabaseOpener(ctx context.Context, open func(ctx context.Context, path string) (*storage.DB, error)) context.Context {
	return context.WithValue(ctx, transport.DatabaseOpenerKey{}, transport.DatabaseOpener(open))
}
