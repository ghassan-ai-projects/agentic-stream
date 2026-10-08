package store

import (
	"context"
	"fmt"

	"github.com/ghassan-ai-projects/agentic-stream/internal/actionport"
	deviceauthority "github.com/ghassan-ai-projects/agentic-stream/internal/authority"
	domain "github.com/ghassan-ai-projects/agentic-stream/internal/runartifact/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
)

// SafetyEvidence reads the device authority's safety tally and the tenant's
// action outcome counts in this snapshot.
func (s *Snapshot) SafetyEvidence(ctx context.Context, tenantID string) (domain.SafetyEvidence, error) {
	record, err := deviceauthority.ReadSafetyRecord(ctx, s.tx)
	if err != nil {
		return domain.SafetyEvidence{}, fmt.Errorf("read device safety record: %w", err)
	}
	diagnostics, err := s.actionDiagnostics(ctx, tenantID)
	if err != nil {
		return domain.SafetyEvidence{}, err
	}
	return domain.SafetyEvidence{
		ZeroTolerance: zeroTolerance(record), PhysicalTransitions: record.PhysicalTransitions,
		CompleteTransitions: record.CompleteTransitions, OpenReconciliations: record.OpenReconciliations,
		AuthorityEvents: record.AuthorityEvents, ActionDiagnostics: diagnostics,
	}, nil
}

func zeroTolerance(record deviceauthority.SafetyRecord) domain.ZeroTolerance {
	counts := record.EventCounts
	return domain.ZeroTolerance{
		UnsafeOutputCount:             counts[deviceauthority.SafetyUnsafeOutput],
		StaleEnergizingEffectCount:    counts[deviceauthority.SafetyStaleEnergizingEffect],
		DuplicateNetEnergizingCount:   counts[deviceauthority.SafetyDuplicateNetEnergizingEffect],
		UnexplainedActuatorTransition: counts[deviceauthority.SafetyUnexplainedActuatorTransition],
		FalseVerifiedSuccessCount:     counts[deviceauthority.SafetyFalseVerifiedSuccess],
		SafeStateDeadlineMissCount:    counts[deviceauthority.SafetySafeStateDeadlineMiss],
	}
}

type countQuery struct {
	name, query string
	args        []any
}

func (s *Snapshot) actionDiagnostics(ctx context.Context, tenantID string) (map[string]uint64, error) {
	counts := make(map[string]uint64)
	for _, item := range tenantCountQueries() {
		var count int64
		if err := s.tx.QueryRowContext(ctx, item.query, append([]any{tenantID}, item.args...)...).Scan(&count); err != nil {
			return nil, fmt.Errorf("count %s: %w", item.name, err)
		}
		if count < 0 {
			return nil, fmt.Errorf("count %s returned a negative value", item.name)
		}
		counts[item.name] = uint64(count)
	}
	return counts, nil
}

func tenantCountQueries() []countQuery {
	unresolved, unresolvedArgs := storage.InClause("c.status", actionport.UnresolvedCommandStatuses())
	awaiting := []any{actionport.VerificationAwaiting}
	return []countQuery{
		{name: "commands", query: "SELECT COUNT(*) FROM commands WHERE tenant_id = ?"},
		{name: "unknown_outcomes", query: "SELECT COUNT(*) FROM commands c WHERE c.tenant_id = ? AND " + unresolved, args: unresolvedArgs},
		{name: "awaiting_verification", query: `SELECT COUNT(*) FROM verifications v JOIN commands c ON c.command_id = v.command_id
				WHERE c.tenant_id = ? AND v.status = ?`, args: awaiting},
		{name: "unresolved_action_outcomes", query: `SELECT COUNT(*) FROM commands c LEFT JOIN verifications v ON v.command_id = c.command_id
				WHERE c.tenant_id = ? AND (` + unresolved + ` OR v.status = ?)`, args: append(unresolvedArgs, awaiting...)},
	}
}
