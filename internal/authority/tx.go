package authority

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// withAdmittedTx runs fn in one transaction whose first step is ordinary
// admission for epoch, so the decision and its writes share the fence.
func (s *Service) withAdmittedTx(ctx context.Context, epoch string, fn func(*sql.Tx) error) error {
	return s.db.WithTx(ctx, func(tx *sql.Tx) error {
		if err := s.admitOrdinary(ctx, tx, epoch); err != nil {
			return err
		}
		return fn(tx)
	})
}

// withPriorityTx runs fn in one transaction without ordinary admission. Only
// the priority path uses it: writes that must succeed whatever the authority
// state, because they make the device safer, give up authority, or record
// safety evidence.
func (s *Service) withPriorityTx(ctx context.Context, fn func(*sql.Tx) error) error {
	return s.db.WithTx(ctx, fn)
}

// admitOrdinary requires the owner's lease to be valid and its epoch to be
// neither draining nor killed.
func (s *Service) admitOrdinary(ctx context.Context, tx *sql.Tx, epoch string) error {
	if err := s.owner.Assert(ctx, tx, epoch); err != nil {
		return fmt.Errorf("runtime authority: %w", err)
	}
	if err := s.epochs.AssertOrdinaryTx(ctx, tx, epoch); err != nil {
		return fmt.Errorf("epoch control: %w", err)
	}
	return nil
}

// now is the operation's single clock read.
func (s *Service) now() time.Time {
	return s.clock.Now().UTC()
}
