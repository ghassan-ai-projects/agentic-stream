package campaign

import (
	"encoding/json"
)

const reportFile = "campaign-score.json"

// RunResult is one graded run: what the ledgers recorded and the simulator's
// grade.
type RunResult struct {
	Index  int             `json:"index"`
	Phase  string          `json:"phase"`
	Ledger Ledger          `json:"ledger"`
	Grade  json.RawMessage `json:"grade"`

	LearningSettled bool `json:"learning_settled,omitempty"`
}

// Report is the campaign score of one arm: every run and the ledgers summed
// per phase.
type Report struct {
	Arm    string            `json:"arm"`
	Runs   []RunResult       `json:"runs"`
	Phases map[string]Ledger `json:"phases"`
}

// Add records a run and adds its ledger to its phase.
func (r *Report) Add(result RunResult) {
	r.Runs = append(r.Runs, result)
	if r.Phases == nil {
		r.Phases = map[string]Ledger{}
	}
	r.Phases[result.Phase] = r.Phases[result.Phase].plus(result.Ledger)
}

// Write writes the report into dir.
func (r Report) Write(dir string) error {
	_, err := writeIn(dir, reportFile, r)
	return err
}

func (l Ledger) plus(o Ledger) Ledger {
	return Ledger{
		ModelEpisodes: l.ModelEpisodes + o.ModelEpisodes, RuleEpisodes: l.RuleEpisodes + o.RuleEpisodes,
		DraftCalls: l.DraftCalls + o.DraftCalls, DraftTokens: l.DraftTokens + o.DraftTokens, CostMicro: l.CostMicro + o.CostMicro,
		Commands: l.Commands + o.Commands, ApprovalRequests: l.ApprovalRequests + o.ApprovalRequests,
		RuleProposals: l.RuleProposals + o.RuleProposals, CircuitsOpened: l.CircuitsOpened + o.CircuitsOpened,
		CasesVerified: l.CasesVerified + o.CasesVerified, CasesNotVerified: l.CasesNotVerified + o.CasesNotVerified,
		RuleDecisionP95MS: max(l.RuleDecisionP95MS, o.RuleDecisionP95MS),
	}
}
