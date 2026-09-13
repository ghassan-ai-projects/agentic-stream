package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/costcontrol"
)

// ErrEpochKilled means the epoch was killed and every later decision under it
// is refused (independently of the worker).
var ErrEpochKilled = errors.New("policy epoch is killed")

// ErrEpochUnbound means a decision has no recorded owner epoch.
var ErrEpochUnbound = errors.New("policy epoch is unbound")

// ErrEpochDraining means the epoch is draining and new episodes are refused
// (in-flight episodes finish under their recorded epoch).
var ErrEpochDraining = errors.New("policy epoch is draining")

// EpochControl is the durable drain/kill record. One row per epoch that has
// been drained or killed. Reading the state is race-free: the runtime process
// that owns the lease is the only writer, and the decision path validates the
// EPISODE's recorded policy_epoch against this table — a killed epoch refuses
// in-flight decisions even if a hostile worker keeps producing them.
type EpochControl struct {
	DB  *DB
	Now func() time.Time
}

// Kill marks the epoch killed: every later decision under it is refused, and
// in-flight (running/admitted) episodes of that epoch are marked superseded so
// the runner's watchSupersession cancels their provider calls.
func (c *EpochControl) Kill(ctx context.Context, epoch string) error {
	if c == nil || c.DB == nil {
		return fmt.Errorf("epoch control is not configured")
	}
	if epoch == "" {
		return fmt.Errorf("epoch control is not configured")
	}
	now := c.now()
	if err := c.DB.WithTx(ctx, func(tx *sql.Tx) error {
		if err := c.setTx(ctx, tx, epoch, "killed", now); err != nil {
			return err
		}
		var admitted []string
		rows, err := tx.QueryContext(ctx, `
			SELECT e.episode_id
			FROM episodes e JOIN cost_reservations r ON r.episode_id = e.episode_id
			WHERE e.policy_epoch = ? AND e.lifecycle_status = 'admitted'
			  AND e.current_attempt_id IS NULL AND r.status = 'reserved'`, epoch)
		if err != nil {
			return fmt.Errorf("list admitted epoch reservations: %w", err)
		}
		for rows.Next() {
			var episodeID string
			if err := rows.Scan(&episodeID); err != nil {
				_ = rows.Close()
				return fmt.Errorf("scan admitted epoch reservation: %w", err)
			}
			admitted = append(admitted, episodeID)
		}
		if err := rows.Err(); err != nil {
			_ = rows.Close()
			return fmt.Errorf("read admitted epoch reservations: %w", err)
		}
		if err := rows.Close(); err != nil {
			return fmt.Errorf("close admitted epoch reservations: %w", err)
		}
		// Cancel in-flight episodes of the killed epoch (gate 2 first half):
		// the runner's supersession watcher turns this into context
		// cancellation of the provider call. The decision gates (pre- and
		// post-execute) refuse any outcome that still lands. Keep this write
		// in the same transaction as the terminal kill record.
		if _, err := tx.ExecContext(ctx, `
			UPDATE episodes SET lifecycle_status = 'superseded', ended_at = ?
			WHERE policy_epoch = ? AND lifecycle_status IN ('admitted', 'running')`,
			formatRuntimeTime(now), epoch); err != nil {
			return fmt.Errorf("supersede in-flight episodes of killed epoch: %w", err)
		}
		for _, episodeID := range admitted {
			if err := (costcontrol.Controller{}).Settle(ctx, tx, episodeID, 0, formatRuntimeTime(now)); err != nil {
				return fmt.Errorf("release admitted episode cost %s: %w", episodeID, err)
			}
		}
		return nil
	}); err != nil {
		return fmt.Errorf("kill epoch %q: %w", epoch, err)
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
		return "", fmt.Errorf("epoch control is not configured")
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
	if c == nil || c.DB == nil {
		return fmt.Errorf("epoch control is not configured")
	}
	if episodeEpoch == "" {
		return ErrEpochUnbound
	}
	state, err := c.State(ctx, episodeEpoch)
	if err != nil {
		return err
	}
	return decisionEpochState(state)
}

// AssertDecisionTx performs the decision-boundary check on an existing
// transaction. Callers that are about to persist a decision use this variant
// so the kill observation and the terminal writes share one SQLite snapshot.
func (c *EpochControl) AssertDecisionTx(ctx context.Context, tx *sql.Tx, episodeEpoch string) error {
	if c == nil || c.DB == nil || tx == nil {
		return fmt.Errorf("epoch control is not configured")
	}
	if episodeEpoch == "" {
		return ErrEpochUnbound
	}
	var state string
	err := tx.QueryRowContext(ctx,
		`SELECT state FROM epoch_control WHERE epoch = ?`, episodeEpoch).Scan(&state)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read epoch control: %w", err)
	}
	return decisionEpochState(state)
}

func decisionEpochState(state string) error {
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
	now := c.now()
	if err := c.DB.WithTx(ctx, func(tx *sql.Tx) error {
		return c.setTx(ctx, tx, epoch, state, now)
	}); err != nil {
		return fmt.Errorf("record epoch control: %w", err)
	}
	return nil
}

func (c *EpochControl) setTx(ctx context.Context, tx *sql.Tx, epoch, state string, now time.Time) error {
	if state != "draining" && state != "killed" {
		return fmt.Errorf("invalid epoch control state %q", state)
	}
	query := epochControlUpsert(state)
	if _, err := tx.ExecContext(ctx, query, epoch, state, formatRuntimeTime(now)); err != nil {
		return fmt.Errorf("record epoch control: %w", err)
	}
	return nil
}

func epochControlUpsert(state string) string {
	const killed = `
		INSERT INTO epoch_control (epoch, state, updated_at) VALUES (?, ?, ?)
		ON CONFLICT(epoch) DO UPDATE SET state = excluded.state, updated_at = excluded.updated_at
		WHERE epoch_control.state <> 'killed'`
	const draining = `
		INSERT INTO epoch_control (epoch, state, updated_at) VALUES (?, ?, ?)
		ON CONFLICT(epoch) DO UPDATE SET state = excluded.state, updated_at = excluded.updated_at
		WHERE epoch_control.state IS NULL OR epoch_control.state <> 'killed'`
	if state == "draining" {
		return draining
	}
	return killed
}

func (c *EpochControl) now() time.Time {
	if c.Now != nil {
		return c.Now().UTC()
	}
	return time.Now().UTC()
}
