package store

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/storage/storagetest"
	"github.com/ghassan-ai-projects/agentic-stream/internal/watch/internal/domain"
)

func allowOwner(context.Context, *sql.Tx, string) error { return nil }

func openStore(t *testing.T) Store {
	t.Helper()
	db := storagetest.OpenTemp(t)

	return New(db, allowOwner, "epoch")
}

func inTx(t *testing.T, s Store, use func(*Tx) error) {
	t.Helper()
	if err := s.WithTx(t.Context(), use); err != nil {
		t.Fatal(err)
	}
}

func condition() domain.Condition {
	return domain.Condition{TenantID: "tenant", SituationID: "sit-1", Expression: "features.x > 1", Target: "motor-1",
		ExpiresAt: testNow.Add(time.Hour), SituationVersion: 1, MaxFires: 2}
}

var testNow = time.Date(2026, 10, 6, 12, 0, 0, 0, time.UTC)
