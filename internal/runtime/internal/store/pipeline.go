package store

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/episodes"
	"github.com/ghassan-ai-projects/agentic-stream/internal/playbook"
	"github.com/ghassan-ai-projects/agentic-stream/internal/policy"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
)

type PipelineStore struct {
	DB           *storage.DB
	Policy       *policy.Service
	RuntimeOwner storage.OwnerCheck
	OwnerEpoch   string
	Episodes     *episodes.Service
	Playbook     *playbook.Service
	TenantID     string
}

func (p *PipelineStore) AssertOwner(ctx context.Context) error {
	if err := p.DB.WithTx(ctx, func(tx *sql.Tx) error {
		return assertRuntimeOwner(ctx, tx, p.RuntimeOwner, p.OwnerEpoch)
	}); err != nil {
		return fmt.Errorf("assert pipeline owner: %w", err)
	}
	return nil
}

func (p *PipelineStore) NextPendingIntent(ctx context.Context, tenant string) (string, bool, error) {
	return policy.NextPendingIntent(ctx, p.DB.DB, tenant) //nolint:wrapcheck // Policy names the failed read; preserve the existing batch error text.
}

func (p *PipelineStore) EvaluateIntent(ctx context.Context, intentID string, now time.Time) error {
	if err := p.DB.WithTx(ctx, func(tx *sql.Tx) error {
		if _, err := p.Policy.EvaluateIntent(ctx, tx, policy.EvaluationRequest{IntentID: intentID, Now: now}); err != nil {
			return fmt.Errorf("evaluate intent: %w", err)
		}
		return nil
	}); err != nil {
		return fmt.Errorf("evaluate intent %s: %w", intentID, err)
	}
	return nil
}
