package authority

import (
	"context"
	"database/sql"

	"github.com/ghassan-ai-projects/agentic-stream/internal/authority/internal/app"
	"github.com/ghassan-ai-projects/agentic-stream/internal/authority/internal/store"
)

// OwnerInstance is the runtime owner instance every operation must name.
func (s *Service) OwnerInstance() string { return s.app.OwnerInstance() }

// AssertRuntime verifies ordinary admission for epoch without touching device
// state.
func (s *Service) AssertRuntime(ctx context.Context, epoch string) error {
	return s.app.AssertRuntime(ctx, epoch)
}

// Claim acquires or renews a target claim. While another owner holds a live
// claim, the attempt is audited and refused with ErrTargetClaimBusy.
func (s *Service) Claim(ctx context.Context, claim TargetClaim) error {
	return s.app.Claim(ctx, claim)
}

// AssertClaim verifies that the exact claim is live and the owner is still
// admitted. Call it immediately before an ordinary transport send: a failure
// after the send is an unknown outcome, because bytes cannot be retracted.
func (s *Service) AssertClaim(ctx context.Context, claim TargetClaim) error {
	return s.app.AssertClaim(ctx, claim)
}

// ReleaseClaim gives up the exact active claim, even after authority loss; a
// stale owner cannot release a replacement claim.
func (s *Service) ReleaseClaim(ctx context.Context, claim TargetClaim) error {
	return s.app.ReleaseClaim(ctx, claim)
}

// BindCommand records the target, device boot and owner of a command
// immediately before delivery. Repeating the identical binding is idempotent;
// a conflicting binding is refused before any bytes are sent.
func (s *Service) BindCommand(ctx context.Context, binding CommandBinding) error {
	return s.app.BindCommand(ctx, binding)
}

// RecordDeviceState records the state a device reported and reports whether
// a reconciliation is required: the first state is clear, the same boot keeps
// its status, and a reboot opens a reconciliation.
func (s *Service) RecordDeviceState(ctx context.Context, owner Owner, document map[string]any) (bool, error) {
	return s.app.RecordDeviceState(ctx, owner, document)
}

// ReconciliationRequired reports whether ordinary commands to the device are
// blocked by an open reconciliation.
func (s *Service) ReconciliationRequired(ctx context.Context, deviceID string) (bool, error) {
	return s.app.ReconciliationRequired(ctx, deviceID)
}

// OpenReconciliation opens a reconciliation for the current device boot, for
// example when a receipt cannot be trusted.
func (s *Service) OpenReconciliation(ctx context.Context, opening ReconciliationOpening) error {
	return s.app.OpenReconciliation(ctx, opening)
}

// OpenReconciliationAfterAuthorityLoss opens the same reconciliation on the
// priority path, after the owner's lease expired or its epoch was fenced.
func (s *Service) OpenReconciliationAfterAuthorityLoss(ctx context.Context, opening ReconciliationOpening) error {
	return s.app.OpenReconciliationAfterAuthorityLoss(ctx, opening)
}

// ResolveReconciliation records evidence and an outcome for the open
// reconciliation of a device boot, and reports whether it cleared.
func (s *Service) ResolveReconciliation(ctx context.Context, request ResolutionRequest) (bool, error) {
	return s.app.ResolveReconciliation(ctx, request)
}

// RecordSafeStop records one safe-stop stage on the priority path. Any
// recorded stage latches the device boot.
func (s *Service) RecordSafeStop(ctx context.Context, claim TargetClaim, stage SafeStopStage, details map[string]any) error {
	return s.app.RecordSafeStop(ctx, claim, stage, details)
}

// SafeStopLatched reports whether a safe stop was recorded for the device
// boot. Only a new boot resets the latch.
func (s *Service) SafeStopLatched(ctx context.Context, device DeviceBoot) (bool, error) {
	return s.app.SafeStopLatched(ctx, device)
}

// RecordSafetyEvent appends one piece of validated safety evidence.
func (s *Service) RecordSafetyEvent(ctx context.Context, event SafetyEvent) error {
	return s.app.RecordSafetyEvent(ctx, event)
}

// VerifyCommandEvidence checks reconciliation evidence for a command inside
// the caller's transaction. Evidence for a device-bound command must name the
// bound target and device boot; an unbound command needs no device evidence.
func VerifyCommandEvidence(ctx context.Context, tx *sql.Tx, presented CommandEvidence) error {
	return app.VerifyCommandEvidence(ctx, store.Join(tx), presented)
}

// ReadSafetyRecord reads the durable safety evidence inside the caller's
// transaction. Stored evidence that no longer matches its digest fails the read.
func ReadSafetyRecord(ctx context.Context, tx *sql.Tx) (SafetyRecord, error) {
	return app.ReadSafetyRecord(ctx, store.Join(tx))
}
