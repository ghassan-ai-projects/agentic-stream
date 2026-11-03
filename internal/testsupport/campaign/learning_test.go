package campaign

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/storage/storagetest"
)

const requestedDraft = `INSERT INTO rule_drafts (draft_id, tenant_id, diagnosis_code, kind_digest, intent, preset, status, evidence_cases,
	created_at, updated_at) VALUES ('rdraft_1', 'default', 'bearing_wear', 'sha256:aa', 'schedule_maintenance', 'bearing_wear', 'requested', 3,
	'2026-01-01T00:00:00.000000000Z', '2026-01-01T00:00:00.000000000Z')`

func playbookWithModes(t *testing.T, modes ...string) string {
	t.Helper()
	text := "rules:\n"
	for _, mode := range modes {
		text += "  - mode: " + mode + "\n"
	}
	path := filepath.Join(t.TempDir(), "site-a.playbook.yaml")
	if err := os.WriteFile(path, []byte(text), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestLearningSettlesOnlyWhenEveryRuleIsActiveAndNothingIsInFlight(t *testing.T) {
	t.Parallel()
	for name, tc := range map[string]struct {
		modes   []string
		inserts string
		want    bool
	}{
		"every rule active":       {modes: []string{"active", "active"}, want: true},
		"a rule still in shadow":  {modes: []string{"active", "shadow"}},
		"no rule yet":             {},
		"a draft still requested": {modes: []string{"active"}, inserts: requestedDraft},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			db := storagetest.OpenTempWithoutForeignKeys(t)
			if tc.inserts != "" {
				if _, err := db.ExecContext(t.Context(), tc.inserts); err != nil {
					t.Fatal(err)
				}
			}
			settled, err := LearningSettled(playbookWithModes(t, tc.modes...), 3)(t.Context(), db.DB)
			if err != nil || settled != tc.want {
				t.Fatalf("settled = %v, %v; want %v", settled, err, tc.want)
			}
		})
	}
}
