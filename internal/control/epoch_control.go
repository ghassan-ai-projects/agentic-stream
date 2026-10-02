package control

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/episodeledger"

	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"

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
	DB  *storage.DB
	Now func() time.Time
}

// Kill marks the epoch killed: every later decision under it is refused, and
// in-flight (running/admitted) episodes of that epoch are marked superseded so
// the runner's watchSupersession cancels their provider calls.
func (c *EpochControl) Kill(ctx context.Context, epoch string) error {
	if c == nil || c.DB == nil || epoch == "" {
		return fmt.Errorf("epoch control is not configured")
	}
	now := c.now()
	if err := c.DB.WithTx(ctx, func(tx *sql.Tx) error {
		return c.killTx(ctx, tx, epoch, now)
	}); err != nil {
		return fmt.Errorf("kill epoch %q: %w", epoch, err)
	}
	return nil
}

// killTx records the kill, cancels the epoch's in-flight episodes, and
// releases the cost reservations of admitted episodes that never started, in
// the transaction that writes the terminal kill record.
func (c *EpochControl) killTx(ctx context.Context, tx *sql.Tx, epoch string, now time.Time) error {
	if err := c.setTx(ctx, tx, epoch, "killed", now); err != nil {
		return err
	}
	// Read the unstarted admitted episodes before supersession rewrites their
	// lifecycle.
	unstarted, err := unstartedReservedEpisodes(ctx, tx, epoch)
	if err != nil {
		return err
	}
	if err := episodeledger.SupersedeEpoch(ctx, tx, epoch, formatRuntimeTime(now)); err != nil {
		return fmt.Errorf("%w", err)
	}
	return releaseEpisodeCosts(ctx, tx, unstarted, now)
}

// unstartedReservedEpisodes lists admitted episodes of the epoch that hold a
// cost reservation but never started an attempt.
func unstartedReservedEpisodes(ctx context.Context, tx *sql.Tx, epoch string) ([]string, error) {
	rows, err := tx.QueryContext(ctx, `
		SELECT e.episode_id
		FROM episodes e JOIN cost_reservations r ON r.episode_id = e.episode_id
		WHERE e.policy_epoch = ? AND e.lifecycle_status = 'admitted'
		  AND e.current_attempt_id IS NULL AND r.status = 'reserved'`, epoch)
	if err != nil {
		return nil, fmt.Errorf("list admitted epoch reservations: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var episodeIDs []string
	for rows.Next() {
		var episodeID string
		if err := rows.Scan(&episodeID); err != nil {
			return nil, fmt.Errorf("scan admitted epoch reservation: %w", err)
		}
		episodeIDs = append(episodeIDs, episodeID)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read admitted epoch reservations: %w", err)
	}
	if err := rows.Close(); err != nil {
		return nil, fmt.Errorf("close admitted epoch reservations: %w", err)
	}
	return episodeIDs, nil
}

func releaseEpisodeCosts(ctx context.Context, tx *sql.Tx, episodeIDs []string, now time.Time) error {
	for _, episodeID := range episodeIDs {
		if err := (costcontrol.Controller{}).Settle(ctx, tx, episodeID, 0, formatRuntimeTime(now)); err != nil {
			return fmt.Errorf("release admitted episode cost %s: %w", episodeID, err)
		}
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
	if _, err := tx.ExecContext(ctx, epochControlUpsert, epoch, state, formatRuntimeTime(now)); err != nil {
		return fmt.Errorf("record epoch control: %w", err)
	}
	return nil
}

// epochControlUpsert records a drain or kill. Kill is terminal: once an epoch
// is killed, neither a later drain nor a repeated kill rewrites the row.
const epochControlUpsert = `
		INSERT INTO epoch_control (epoch, state, updated_at) VALUES (?, ?, ?)
		ON CONFLICT(epoch) DO UPDATE SET state = excluded.state, updated_at = excluded.updated_at
		WHERE epoch_control.state <> 'killed'`

func (c *EpochControl) now() time.Time {
	if c.Now != nil {
		return c.Now().UTC()
	}
	return time.Now().UTC()
}
