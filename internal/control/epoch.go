package control

import (
	"context"
	"database/sql"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/control/internal/app"
	"github.com/ghassan-ai-projects/agentic-stream/internal/control/internal/store"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
)

// EpochControl is the durable drain/kill record: one row per epoch that has
// been drained or killed. The runtime process that owns the lease is the only
// writer, and the decision path validates the episode's recorded policy epoch
// against this table, so a killed epoch refuses in-flight decisions even if a
// hostile worker keeps producing them. An unconfigured value refuses every
// operation.
type EpochControl struct {
	DB  *storage.DB
	Now func() time.Time
}

func (c *EpochControl) session() *app.Epochs {
	if c == nil {
		return nil
	}
	return &app.Epochs{Store: store.New(c.DB), Now: utcNow(c.Now)}
}

// Kill marks the epoch killed: every later decision under it is refused, and
// in-flight (running/admitted) episodes of that epoch are marked superseded so
// the runner cancels their provider calls.
func (c *EpochControl) Kill(ctx context.Context, epoch string) error {
	return app.Kill(ctx, c.session(), epoch)
}

// Drain marks the epoch draining: new episodes are refused at admission.
func (c *EpochControl) Drain(ctx context.Context, epoch string) error {
	return app.Drain(ctx, c.session(), epoch)
}

// State returns "draining", "killed", or "" when the epoch is uncontrolled.
func (c *EpochControl) State(ctx context.Context, epoch string) (string, error) {
	return app.State(ctx, c.session(), epoch)
}

// AssertDecision refuses a decision under a killed epoch; the episode's
// recorded policy epoch is checked.
func (c *EpochControl) AssertDecision(ctx context.Context, episodeEpoch string) error {
	return app.AssertDecision(ctx, c.session(), episodeEpoch)
}

// AssertDecisionTx performs the decision-boundary check on an existing
// transaction so the kill observation and the terminal writes share one SQLite
// snapshot.
func (c *EpochControl) AssertDecisionTx(ctx context.Context, tx *sql.Tx, episodeEpoch string) error {
	return app.AssertDecisionTx(ctx, c.session(), store.Join(tx), episodeEpoch)
}

// AssertOrdinaryTx refuses ordinary action work for an epoch that is draining
// or killed, on the caller's transaction.
func (c *EpochControl) AssertOrdinaryTx(ctx context.Context, tx *sql.Tx, epoch string) error {
	return app.AssertOrdinaryTx(ctx, c.session(), store.Join(tx), epoch)
}

// AssertAdmission refuses NEW episodes while draining or killed.
func (c *EpochControl) AssertAdmission(ctx context.Context, currentEpoch string) error {
	return app.AssertAdmission(ctx, c.session(), currentEpoch)
}
