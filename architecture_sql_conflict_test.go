package agenticstream

import "testing"

func TestInsertConflictClassifierDistinguishesIgnoreFromRewrite(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		query    string
		rewrites bool
	}{
		{`INSERT INTO commands(command_id) VALUES (?)`, false},
		{`INSERT OR IGNORE INTO commands(command_id) VALUES (?)`, false},
		{`INSERT INTO commands(command_id) VALUES ('DO UPDATE SET') ON CONFLICT DO NOTHING`, false},
		{`INSERT INTO commands(command_id) VALUES (?) ON CONFLICT DO UPDATE SET command_json=?`, true},
		{`INSERT OR REPLACE INTO main.commands(command_id) VALUES (?)`, true},
		{`REPLACE INTO "commands"(command_id) VALUES (?)`, true},
	} {
		mutations := sqlMutations(tc.query)
		if len(mutations) != 1 || mutations[0].rewritesExisting != tc.rewrites {
			t.Fatalf("conflict rewrite classification: %s: %+v", tc.query, mutations)
		}
		if allowed := ownsMutation("internal/policy", mutations[0]); allowed == tc.rewrites {
			t.Fatalf("prepared command creation permission: %s: allowed=%v", tc.query, allowed)
		}
	}
}
