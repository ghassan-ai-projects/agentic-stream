package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// ErrEpochKilled means the epoch was killed and every later decision under it
// is refused (independently of the worker).
var ErrEpochKilled = errors.New("policy epoch is killed")

// ErrEpochDraining means the epoch is draining and new episodes are refused
// (in-flight episodes finish under their recorded epoch).
var ErrEpochDraining = errors.New("policy epoch is draining")

// EpochControl is the durable drain/kill record. One row per epoch that has
// been drained or killed. Reading the state is race-free: the runtime process
// that owns the lease is the only writer, and the decision path validates the
// EPISODE's recorded policy_epoch against this table — a killed epoch refuses
// in-flight decisions even if a hostile worker keeps producing them.
type EpochControl struct {
	DB *DB
}

// Kill marks the epoch killed: every later decision under it is refused, and
// in-flight (running/admitted) episodes of that epoch are marked superseded so
// the runner's watchSupersession cancels their provider calls.
func (c *EpochControl) Kill(ctx context.Context, epoch string) error {
	if c == nil || c.DB == nil {
		return fmt.Errorf("epoch control is not configured")
	}
	if err := c.set(ctx, epoch, "killed"); err != nil {
		return err
	}
	// Cancel in-flight episodes of the killed epoch (gate 2 first half): the
	// runner's supersession watcher turns this into context cancellation of
	// the provider call. The decision gates (pre- and post-execute) refuse any
	// outcome that still lands.
	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, err := c.DB.ExecContext(ctx, `
		UPDATE episodes SET lifecycle_status = 'superseded', ended_at = ?
		WHERE policy_epoch = ? AND lifecycle_status IN ('admitted', 'running')`,
		now, epoch)
	if err != nil {
		return fmt.Errorf("supersede in-flight episodes of killed epoch: %w", err)
	}
	return nil
}

// Drain marks the epoch draining: new episodes are refused at admission.
func (c *EpochControl) Drain(ctx context.Context, epoch string) error {
	return c.set(ctx, epoch, "draining")
}

// State returns "draining", "killed", or "" when the epoch is uncontrolled.
func (c *EpochControl) State(ctx context.Context, epoch string) (string, error) {
	if c == nil || c.DB == nil || epoch == "" {
		return "", nil
	}
	var state string
	err := c.DB.QueryRowContext(ctx,
		`SELECT state FROM epoch_control WHERE epoch = ?`, epoch).Scan(&state)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("read epoch control: %w", err)
	}
	return state, nil
}

// AssertDecision refuses a decision under a killed epoch. It is the
// decision-boundary gate: the episode's RECORDED policy_epoch is checked, so
// a kill refuses in-flight episodes even when the worker keeps producing.
func (c *EpochControl) AssertDecision(ctx context.Context, episodeEpoch string) error {
	state, err := c.State(ctx, episodeEpoch)
	if err != nil {
		return err
	}
	if state == "killed" {
		return ErrEpochKilled
	}
	return nil
}

// AssertOrdinaryTx refuses ordinary action work for an epoch that is draining
// or killed. It is intentionally transaction-scoped so target claims and the
// runtime owner assertion share one SQLite read boundary.
func (c *EpochControl) AssertOrdinaryTx(ctx context.Context, tx *sql.Tx, epoch string) error {
	if c == nil || c.DB == nil || tx == nil || epoch == "" {
		return fmt.Errorf("epoch control is not configured")
	}
	var state string
	err := tx.QueryRowContext(ctx, `SELECT state FROM epoch_control WHERE epoch = ?`, epoch).Scan(&state)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read ordinary epoch control: %w", err)
	}
	switch state {
	case "killed":
		return ErrEpochKilled
	case "draining":
		return ErrEpochDraining
	default:
		return fmt.Errorf("unknown epoch control state %q", state)
	}
}

// AssertAdmission refuses NEW episodes while draining or killed.
func (c *EpochControl) AssertAdmission(ctx context.Context, currentEpoch string) error {
	state, err := c.State(ctx, currentEpoch)
	if err != nil {
		return err
	}
	switch state {
	case "killed", "draining":
		return ErrEpochDraining
	default:
		return nil
	}
}

func (c *EpochControl) set(ctx context.Context, epoch, state string) error {
	if c == nil || c.DB == nil || epoch == "" {
		return fmt.Errorf("epoch control is not configured")
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	// Kill is terminal: a later drain must never downgrade a killed epoch back
	// to draining (which would silently re-open in-flight decisions).
	where := `ON CONFLICT(epoch) DO UPDATE SET state = excluded.state, updated_at = excluded.updated_at
	          WHERE epoch_control.state <> 'killed'`
	if state == "draining" {
		where = `ON CONFLICT(epoch) DO UPDATE SET state = excluded.state, updated_at = excluded.updated_at
		          WHERE epoch_control.state IS NULL OR epoch_control.state <> 'killed'`
	}
	query := fmt.Sprintf(`
		INSERT INTO epoch_control (epoch, state, updated_at) VALUES (?, ?, ?)
		%s`, where)
	_, err := c.DB.ExecContext(ctx, query, epoch, state, now)
	if err != nil {
		return fmt.Errorf("record epoch control: %w", err)
	}
	return nil
}
