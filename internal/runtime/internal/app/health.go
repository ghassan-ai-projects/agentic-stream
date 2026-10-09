package app

import (
	"context"
	"fmt"

	"github.com/ghassan-ai-projects/agentic-stream/internal/runtime/internal/store"
)

func (p *Pipeline) HealthGauges(ctx context.Context) (map[string]float64, error) {
	counts, err := store.HealthStore{DB: p.transactions.DB, TenantID: p.tenantID}.Counts(ctx)
	if err != nil {
		return nil, fmt.Errorf("runtime health gauges: %w", err)
	}
	return counts.Gauges(), nil
}
