package app

import (
	"context"
	"errors"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/playbook/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/playbook/internal/store"
)

var ErrCloseReasonRequired = errors.New("closing a rule circuit needs a reason")

func Circuits(ctx context.Context, st store.Store, tenantID string) ([]domain.CircuitView, error) {
	var circuits []domain.CircuitView
	err := st.WithTx(ctx, func(tx *store.Tx) error {
		var err error
		circuits, err = tx.Circuits(ctx, tenantID)
		return err
	})
	return circuits, err
}

func CloseCircuit(ctx context.Context, st store.Store, fence store.Fence, tenantID string, rule domain.RuleKey, reason string, now time.Time) error {
	if reason == "" {
		return ErrCloseReasonRequired
	}
	return st.WithFencedTx(ctx, fence, func(tx *store.Tx) error {
		return tx.CloseCircuit(ctx, tenantID, rule, reason, now.UTC())
	})
}
