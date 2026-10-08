package store

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/ghassan-ai-projects/agentic-stream/internal/actions/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
)

func Reader(db *storage.DB) Store { return Store{db: db} }

func (s Store) IntentCommands(ctx context.Context, intentID string) ([]domain.CommandView, error) {
	commands, err := s.intentCommandRows(ctx, intentID)
	if err != nil {
		return nil, err
	}
	for i := range commands {
		if commands[i].Outcomes, err = s.outcomeViews(ctx, commands[i].CommandID); err != nil {
			return nil, err
		}
		if commands[i].Verifications, err = s.verificationViews(ctx, commands[i].CommandID); err != nil {
			return nil, err
		}
	}
	return commands, nil
}

func (s Store) intentCommandRows(ctx context.Context, intentID string) ([]domain.CommandView, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT command_id, effector_route, normalized_target, status, created_at, updated_at
		FROM commands WHERE intent_id = ? ORDER BY created_at, command_id`, intentID)
	if err != nil {
		return nil, fmt.Errorf("read intent commands: %w", err)
	}
	defer func() { _ = rows.Close() }()
	commands, err := storage.CollectRows(rows, "intent commands", scanCommandView)
	if err != nil {
		return nil, fmt.Errorf("read intent commands: %w", err)
	}
	return commands, nil
}

func scanCommandView(rows *sql.Rows) (domain.CommandView, error) {
	var c domain.CommandView
	if err := rows.Scan(&c.CommandID, &c.EffectorRoute, &c.Target, &c.Status, &c.CreatedAt, &c.UpdatedAt); err != nil {
		return domain.CommandView{}, fmt.Errorf("scan intent command: %w", err)
	}
	return c, nil
}

func (s Store) outcomeViews(ctx context.Context, commandID string) ([]domain.OutcomeView, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT outcome_id, ordinal, status, COALESCE(reconciliation_status, ''), provider_result_json, occurred_at
		FROM outcomes WHERE command_id = ? ORDER BY ordinal`, commandID)
	if err != nil {
		return nil, fmt.Errorf("read command outcomes: %w", err)
	}
	defer func() { _ = rows.Close() }()
	outcomes, err := storage.CollectRows(rows, "command outcomes", scanOutcomeView)
	if err != nil {
		return nil, fmt.Errorf("read command outcomes: %w", err)
	}
	return outcomes, nil
}

func scanOutcomeView(rows *sql.Rows) (domain.OutcomeView, error) {
	var o domain.OutcomeView
	var providerResult []byte
	if err := rows.Scan(&o.OutcomeID, &o.Ordinal, &o.Status, &o.ReconciliationStatus, &providerResult, &o.OccurredAt); err != nil {
		return domain.OutcomeView{}, fmt.Errorf("scan command outcome: %w", err)
	}
	if len(providerResult) > 0 {
		o.ProviderResult = providerResult
	}
	return o, nil
}

func (s Store) verificationViews(ctx context.Context, commandID string) ([]domain.VerificationView, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT verification_id, COALESCE(outcome_id, ''), status, verdict_json, updated_at
		FROM verifications WHERE command_id = ? ORDER BY updated_at, verification_id`, commandID)
	if err != nil {
		return nil, fmt.Errorf("read command verifications: %w", err)
	}
	defer func() { _ = rows.Close() }()
	verifications, err := storage.CollectRows(rows, "command verifications", scanVerificationView)
	if err != nil {
		return nil, fmt.Errorf("read command verifications: %w", err)
	}
	return verifications, nil
}

func scanVerificationView(rows *sql.Rows) (domain.VerificationView, error) {
	var v domain.VerificationView
	var verdict []byte
	if err := rows.Scan(&v.VerificationID, &v.OutcomeID, &v.Status, &verdict, &v.UpdatedAt); err != nil {
		return domain.VerificationView{}, fmt.Errorf("scan command verification: %w", err)
	}
	if len(verdict) > 0 {
		v.Verdict = verdict
	}
	return v, nil
}
