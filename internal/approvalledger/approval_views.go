package approvalledger

import (
	"context"
	"database/sql"

	"github.com/ghassan-ai-projects/agentic-stream/internal/approvalledger/internal/app"
	"github.com/ghassan-ai-projects/agentic-stream/internal/approvalledger/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/approvalledger/internal/store"
)

// ApprovalView is one approval request of an intent and how it ended.
type ApprovalView = domain.ApprovalView

// Approvals reads every approval request of one intent, oldest first. It only
// reads.
func Approvals(ctx context.Context, db *sql.DB, intentID string) ([]ApprovalView, error) {
	return app.Approvals(ctx, store.NewReader(db), intentID)
}
