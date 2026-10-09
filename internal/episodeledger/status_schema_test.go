package episodeledger_test

import (
	"regexp"
	"strings"
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/episodeledger"
	"github.com/ghassan-ai-projects/agentic-stream/internal/episodeledger/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage/storagetest"
)

func TestStatusEnumsMatchTheSchemaChecks(t *testing.T) {
	db := storagetest.OpenTemp(t)
	for _, test := range []struct{ table, column, want string }{
		{"episodes", "lifecycle_status", domain.LifecycleSQL(func(domain.LifecycleStatus) bool { return true })},
		{"episode_attempts", "status", episodeledger.AttemptSQL(func(episodeledger.AttemptStatus) bool { return true })},
	} {
		var ddl string
		if err := db.QueryRowContext(t.Context(), "SELECT sql FROM sqlite_master WHERE type = 'table' AND name = ?", test.table).Scan(&ddl); err != nil {
			t.Fatal(err)
		}
		check := regexp.MustCompile(`(?s)` + test.column + `\s+TEXT NOT NULL CHECK \(\s*` + test.column + ` IN \((.*?)\)\s*\)`).FindStringSubmatch(ddl)
		if check == nil {
			t.Fatalf("%s.%s has no status CHECK in %s", test.table, test.column, ddl)
		}
		got := "(" + strings.Join(strings.Fields(strings.ReplaceAll(check[1], ",", ", ")), " ") + ")"
		if got != test.want {
			t.Errorf("%s.%s CHECK = %s, Go statuses = %s", test.table, test.column, got, test.want)
		}
	}
}
