package campaign

import (
	"context"
	"database/sql"
	"fmt"
	"slices"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/kernel"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
)

// Ledger is what this repository's ledgers record from a point in wall time:
// model and rule work, its cost, the actions and the human requests.
type Ledger struct {
	ModelEpisodes     int     `json:"model_episodes"`
	RuleEpisodes      int     `json:"rule_episodes"`
	DraftCalls        int     `json:"draft_calls"`
	DraftTokens       int     `json:"draft_tokens"`
	CostMicro         int     `json:"cost_micro"`
	Commands          int     `json:"commands"`
	ApprovalRequests  int     `json:"approval_requests"`
	RuleProposals     int     `json:"rule_proposals"`
	CircuitsOpened    int     `json:"circuits_opened"`
	CasesVerified     int     `json:"cases_verified"`
	CasesNotVerified  int     `json:"cases_not_verified"`
	RuleDecisionP95MS float64 `json:"rule_decision_p95_ms"`
}

// RuleShare is the share of decided episodes a rule answered.
func (l Ledger) RuleShare() float64 {
	if l.ModelEpisodes+l.RuleEpisodes == 0 {
		return 0
	}
	return float64(l.RuleEpisodes) / float64(l.ModelEpisodes+l.RuleEpisodes)
}

const ledgerQuery = `SELECT
	(SELECT COUNT(*) FROM episodes WHERE executor_name != 'rule' AND dispatch_policy != 'shadow' AND accepted_at >= ?1),
	(SELECT COUNT(*) FROM episodes WHERE executor_name = 'rule' AND accepted_at >= ?1),
	(SELECT COALESCE(SUM(attempts), 0) FROM rule_drafts WHERE updated_at >= ?1),
	(SELECT COALESCE(SUM(input_tokens + output_tokens), 0) FROM rule_drafts WHERE updated_at >= ?1),
	(SELECT COALESCE(SUM(actual_micro), 0) FROM cost_reservations WHERE created_at >= ?1)
		+ (SELECT COALESCE(SUM(cost_microunits), 0) FROM rule_drafts WHERE updated_at >= ?1),
	(SELECT COUNT(*) FROM commands WHERE created_at >= ?1),
	(SELECT COUNT(*) FROM approvals WHERE requested_at >= ?1),
	(SELECT COUNT(*) FROM rule_proposals WHERE created_at >= ?1),
	(SELECT COUNT(*) FROM rule_circuits WHERE opened_at >= ?1),
	(SELECT COUNT(*) FROM case_verifications WHERE verdict = 'verified' AND created_at >= ?1),
	(SELECT COUNT(*) FROM case_verifications WHERE verdict = 'not_verified' AND created_at >= ?1)`

const ruleLatencyQuery = `SELECT (julianday(d.created_at) - julianday(v.created_at)) * 86400000.0
	FROM decisions d JOIN episodes e ON e.episode_id = d.episode_id
	JOIN situation_versions v ON v.situation_id = d.situation_id AND v.version = d.situation_version
	WHERE e.executor_name = 'rule' AND d.validation_status = 'accepted' AND e.accepted_at >= ?1`

// ReadLedger reads the ledgers from since on.
func ReadLedger(ctx context.Context, db *sql.DB, since time.Time) (Ledger, error) {
	from := kernel.FormatTime(since)
	var l Ledger
	err := db.QueryRowContext(ctx, ledgerQuery, from).Scan(&l.ModelEpisodes, &l.RuleEpisodes, &l.DraftCalls, &l.DraftTokens, &l.CostMicro,
		&l.Commands, &l.ApprovalRequests, &l.RuleProposals, &l.CircuitsOpened, &l.CasesVerified, &l.CasesNotVerified)
	if err != nil {
		return Ledger{}, fmt.Errorf("read the ledgers: %w", err)
	}
	l.RuleDecisionP95MS, err = ruleDecisionP95(ctx, db, from)
	return l, err
}

func ruleDecisionP95(ctx context.Context, db *sql.DB, from string) (float64, error) {
	latencies, err := storage.QueryAll(ctx, db, "rule decision latency", scanOne[float64], ruleLatencyQuery, from)
	if err != nil {
		return 0, fmt.Errorf("read the rule decision latencies: %w", err)
	}
	return percentile95(latencies), nil
}

func percentile95(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	slices.Sort(values)
	return values[(len(values)*95+99)/100-1]
}
