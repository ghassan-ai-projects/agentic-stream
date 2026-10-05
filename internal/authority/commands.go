package authority

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/authority/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/authority/internal/store"
)

// BindCommand records the target, device boot and owner of a command
// immediately before delivery. Repeating the identical binding is
// idempotent; a conflicting binding is refused before any bytes are sent.
func (s *Service) BindCommand(ctx context.Context, binding CommandBinding) error {
	if !binding.Complete() {
		return errors.New("command, target, device, boot and owner are required for a command binding")
	}
	if err := s.checkOwner(binding.Owner); err != nil {
		return err
	}
	now := s.now()
	err := s.withAdmittedTx(ctx, binding.Owner.Epoch, func(tx *sql.Tx) error {
		return bindCommand(ctx, tx, binding, now)
	})
	if err != nil {
		return fmt.Errorf("bind command %q: %w", binding.CommandID, err)
	}
	return nil
}

func bindCommand(ctx context.Context, tx *sql.Tx, binding CommandBinding, now time.Time) error {
	existing, err := store.LoadBinding(ctx, tx, binding.CommandID)
	if err != nil {
		return err
	}
	alreadyBound, err := domain.DecideBinding(existing, binding)
	if err != nil || alreadyBound {
		return err
	}
	return store.InsertBinding(ctx, tx, binding, now)
}

// VerifyCommandEvidence checks reconciliation evidence for a command inside
// the caller's transaction. Evidence for a device-bound command must name the
// bound target and device boot; a command that was never bound to a device
// needs no device evidence.
func VerifyCommandEvidence(ctx context.Context, tx *sql.Tx, commandID, target string, evidence map[string]any) error {
	binding, err := store.LoadBinding(ctx, tx, commandID)
	if err != nil {
		return err
	}
	return domain.CheckCommandEvidence(binding, commandID, target, evidence)
}
