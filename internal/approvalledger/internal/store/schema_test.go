package store_test

import (
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/approvalledger/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage/storagetest"
)

func TestApprovalStatusesMatchTheSchemaCheck(t *testing.T) {
	t.Parallel()
	db := storagetest.OpenTemp(t)
	var ddl string
	if err := db.QueryRowContext(t.Context(), "SELECT sql FROM sqlite_master WHERE type = 'table' AND name = 'approvals'").Scan(&ddl); err != nil {
		t.Fatal(err)
	}
	check := regexp.MustCompile(`(?s)status\s+TEXT NOT NULL CHECK \(\s*status IN \((.*?)\)\s*\)`).FindStringSubmatch(ddl)
	if check == nil {
		t.Fatalf("approvals.status has no CHECK in %s", ddl)
	}
	var stored []string
	for _, status := range strings.Split(check[1], ",") {
		stored = append(stored, strings.Trim(strings.TrimSpace(status), "'"))
	}
	slices.Sort(stored)
	want := []string{domain.StatusApproved, domain.StatusDenied, domain.StatusExpired, domain.StatusPending}
	slices.Sort(want)
	if !slices.Equal(stored, want) {
		t.Fatalf("approvals.status CHECK = %v, Go statuses = %v", stored, want)
	}
}
