package main

import (
	"errors"
	"strings"
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/playbook"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage/storagetest"
)

func TestAnOperatorClosesARuleCircuitToSwitchTheRuleBackOn(t *testing.T) {
	t.Parallel()
	path := newMigratedDatabasePath(t)
	db, err := storagetest.Open(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.ExecContext(t.Context(), `INSERT INTO rule_circuits (rule_id, rule_version, tenant_id, state, cause, evidence_json, opened_at)
		VALUES ('bearing-wear', 1, 'default', 'open', 'rate_cap', CAST('{}' AS BLOB), '2026-01-01T00:00:00.000000000Z')`)
	if closeErr := db.Close(); err != nil || closeErr != nil {
		t.Fatal(err, closeErr)
	}
	if out, err := runOperatorCommand(t, "circuits", "list", "--db", path); err != nil || !strings.Contains(out, "bearing-wear v1 open rate_cap") {
		t.Fatalf("circuits list = %q, %v; want the open circuit", out, err)
	}
	if _, err := runOperatorCommand(t, "circuits", "close", "--db", path, "--rule", "bearing-wear", "--version", "1"); !errors.Is(err, playbook.ErrCloseReasonRequired) {
		t.Fatalf("close without a reason = %v, want it refused", err)
	}
	close := []string{"circuits", "close", "--db", path, "--rule", "bearing-wear", "--version", "1", "--reason", "rate cap raised by review"}
	if _, err := runOperatorCommand(t, close...); err != nil {
		t.Fatal(err)
	}
	if out, err := runOperatorCommand(t, "circuits", "list", "--db", path, "--json"); err != nil || !strings.Contains(out, `"state":"closed"`) {
		t.Fatalf("circuits list after close = %q, %v; want it closed", out, err)
	}
	if _, err := runOperatorCommand(t, close...); !errors.Is(err, playbook.ErrNoOpenCircuit) {
		t.Fatalf("closing a closed circuit = %v, want no open circuit", err)
	}
}
