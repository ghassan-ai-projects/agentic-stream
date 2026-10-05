package app

import (
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/policy/internal/domain"
)

type evaluation struct {
	row            domain.IntentRecord
	documents      domain.GovernanceDocuments
	result         domain.Result
	now, expiresAt time.Time
}

func newEvaluation(row domain.IntentRecord, now time.Time) evaluation {
	return evaluation{row: row, now: now, result: domain.Result{IntentID: row.IntentID, DecisionID: row.DecisionID}}
}
