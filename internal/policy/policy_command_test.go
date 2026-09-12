package policy

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/ids"
	"github.com/ghassan-ai-projects/agentic-stream/internal/interlock"
)

type stubInterlock struct {
	err error
}

func (s stubInterlock) Assert(context.Context, *sql.Tx, string, string, string) error {
	return s.err
}

func TestGatewayPropagatesInterlockInfrastructureFailure(t *testing.T) {
	ctx := context.Background()
	db, intentID := openPolicyFixture(t, "R1", 1, 1, time.Date(2099, 1, 1, 0, 0, 0, 0, time.UTC))
	defer func() { _ = db.Close() }()
	wantErr := errors.New("temporary interlock storage failure")
	gateway := NewGateway("policy-v1", ids.Deterministic()).WithInterlock(stubInterlock{err: wantErr})

	err := db.WithTx(ctx, func(tx *sql.Tx) error {
		_, err := gateway.EvaluateIntent(ctx, tx, intentID, time.Date(2026, 8, 12, 12, 0, 0, 0, time.UTC))
		return err
	})
	if err == nil || !errors.Is(err, wantErr) {
		t.Fatalf("interlock infrastructure error = %v, want wrapped %v", err, wantErr)
	}

	var status string
	if err := db.QueryRowContext(ctx, "SELECT policy_status FROM intents WHERE intent_id = ?", intentID).Scan(&status); err != nil {
		t.Fatalf("load intent status: %v", err)
	}
	if status != "pending" {
		t.Fatalf("intent status = %q, want pending after retryable failure", status)
	}
	var evaluations, commands int
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM policy_evaluations WHERE intent_id = ?", intentID).Scan(&evaluations); err != nil {
		t.Fatalf("count policy evaluations: %v", err)
	}
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM commands WHERE intent_id = ?", intentID).Scan(&commands); err != nil {
		t.Fatalf("count commands: %v", err)
	}
	if evaluations != 0 || commands != 0 {
		t.Fatalf("retryable interlock failure persisted evaluations=%d commands=%d", evaluations, commands)
	}
}

func TestGatewayRecordsInterlockDenialCauseWithoutChangingStableReason(t *testing.T) {
	ctx := context.Background()
	db, intentID := openPolicyFixture(t, "R1", 1, 1, time.Date(2099, 1, 1, 0, 0, 0, 0, time.UTC))
	defer func() { _ = db.Close() }()
	gateway := NewGateway("policy-v1", ids.Deterministic()).WithInterlock(stubInterlock{
		err: fmt.Errorf("%w: operator stop", interlock.ErrTripped),
	})
	var result Result
	if err := db.WithTx(ctx, func(tx *sql.Tx) error {
		var err error
		result, err = gateway.EvaluateIntent(ctx, tx, intentID, time.Date(2026, 8, 12, 12, 0, 0, 0, time.UTC))
		return err
	}); err != nil {
		t.Fatalf("evaluate tripped interlock: %v", err)
	}
	if result.Result != "denied" || result.Reason != "interlock_not_ready" {
		t.Fatalf("result = %+v, want denied/interlock_not_ready", result)
	}
	var auditReason string
	if err := db.QueryRowContext(ctx, "SELECT reason FROM policy_evaluations WHERE intent_id = ?", intentID).Scan(&auditReason); err != nil {
		t.Fatalf("load interlock audit: %v", err)
	}
	if !strings.HasPrefix(auditReason, "interlock_not_ready: ") || !strings.Contains(auditReason, "operator stop") {
		t.Fatalf("audit reason = %q, want stable code with concrete cause", auditReason)
	}
}

func TestRateLimitDenialDoesNotConsumeDispatchBudget(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 8, 14, 12, 0, 0, 0, time.UTC)
	db, intentID := openPolicyFixture(t, "R1", 1, 1, time.Date(2099, 1, 1, 0, 0, 0, 0, time.UTC))
	defer func() { _ = db.Close() }()
	if _, err := db.ExecContext(ctx, "UPDATE intents SET rate_limit_per_hour = 1 WHERE intent_id = ?", intentID); err != nil {
		t.Fatalf("set rate limit: %v", err)
	}
	bucket := now.UTC().Format("2006-01-02T15:00")
	if _, err := db.ExecContext(ctx, `
		INSERT INTO intent_dispatch_counts (tenant_id, intent_type, bucket, count)
		VALUES ('tenant', 'create_ticket', ?, 1)`, bucket); err != nil {
		t.Fatalf("seed dispatch count: %v", err)
	}

	var result Result
	if err := db.WithTx(ctx, func(tx *sql.Tx) error {
		var err error
		result, err = NewGateway("policy-v1", ids.Deterministic()).EvaluateIntent(ctx, tx, intentID, now)
		return err
	}); err != nil {
		t.Fatalf("evaluate rate-limited intent: %v", err)
	}
	if result.Result != "denied" || result.Reason != "rate_limited" {
		t.Fatalf("result = %+v, want denied/rate_limited", result)
	}
	var count, commands, outbox int
	if err := db.QueryRowContext(ctx, "SELECT count FROM intent_dispatch_counts WHERE tenant_id = 'tenant' AND intent_type = 'create_ticket' AND bucket = ?", bucket).Scan(&count); err != nil {
		t.Fatalf("load dispatch count: %v", err)
	}
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM commands WHERE intent_id = ?", intentID).Scan(&commands); err != nil {
		t.Fatalf("count commands: %v", err)
	}
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM outbox WHERE kind = 'command' AND aggregate_id IN (SELECT command_id FROM commands WHERE intent_id = ?)", intentID).Scan(&outbox); err != nil {
		t.Fatalf("count command outbox: %v", err)
	}
	if count != 1 || commands != 0 || outbox != 0 {
		t.Fatalf("rate-limited side effects count=%d commands=%d outbox=%d, want 1/0/0", count, commands, outbox)
	}
}
