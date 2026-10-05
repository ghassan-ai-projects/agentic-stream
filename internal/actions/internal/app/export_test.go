package app

import "context"

// ReconcileUnknown exposes the private reconciliation use case to the
// behavior tests; production reaches it only through DispatchOnce.
func (s *Service) ReconcileUnknown(ctx context.Context, commandID, finalStatus string, evidence map[string]any) error {
	return s.reconcileUnknown(ctx, commandID, finalStatus, evidence)
}
