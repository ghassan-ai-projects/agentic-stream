package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/ghassan-ai-projects/agentic-stream/internal/api"
	"github.com/ghassan-ai-projects/agentic-stream/internal/policy"
)

func (core *runtimeCore) presentApproval(ctx context.Context, r api.ApprovalSelection) (any, error) {
	result, err := core.pipeline.ApprovalForSigning(ctx, policy.ApprovalLookup{ID: r.ID, Approver: r.Approver, Relay: r.Relay, Approved: r.Approved})
	if err != nil {
		return nil, fmt.Errorf("present operator approval: %w", err)
	}
	return result, nil
}

func (core *runtimeCore) resolveApproval(ctx context.Context, r api.ApprovalSubmission) (any, error) {
	result, err := core.pipeline.ResolveApproval(ctx, policy.ApprovalResolution{ID: r.ID, Approver: r.Approver, Relay: r.Relay, Approved: r.Approved, Signature: r.Signature, Reason: r.Reason})
	if err != nil {
		return nil, fmt.Errorf("resolve operator approval: %w", err)
	}
	return result, nil
}

func approvalErrorStatus(err error) int {
	switch {
	case errors.Is(err, policy.ErrApprovalNotFound):
		return http.StatusNotFound
	case errors.Is(err, policy.ErrApprovalUnauthorized):
		return http.StatusForbidden
	case errors.Is(err, policy.ErrApprovalResolved):
		return http.StatusConflict
	default:
		return http.StatusServiceUnavailable
	}
}
