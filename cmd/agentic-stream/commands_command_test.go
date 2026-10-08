package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCommandsListAndResolveRefuseWhatIsNotAwaiting(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	db := filepath.Join(dir, "runtime.db")
	if out, err := runOperatorCommand(t, "commands", "list", "--db", db); err != nil || !strings.Contains(out, "none awaiting reconciliation") {
		t.Fatalf("list = %q, %v", out, err)
	}
	evidence := filepath.Join(dir, "evidence.json")
	if err := os.WriteFile(evidence, []byte(`{"source":"operator","evidence_type":"provider_observation","provider_id":"p-1","evidence_digest":"sha256:0000000000000000000000000000000000000000000000000000000000000000"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := runOperatorCommand(t, "commands", "resolve", "cmd-missing", "--db", db, "--status", "succeeded", "--evidence", evidence); err == nil {
		t.Fatal("an unknown command was resolved")
	}
	if _, err := runOperatorCommand(t, "commands", "resolve", "cmd-missing", "--db", db, "--status", "succeeded"); err == nil || !strings.Contains(err.Error(), "--evidence is required") {
		t.Fatalf("resolve without evidence = %v", err)
	}
}
