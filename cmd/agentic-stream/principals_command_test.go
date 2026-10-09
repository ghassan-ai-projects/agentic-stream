package main

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	runtimecontrol "github.com/ghassan-ai-projects/agentic-stream/internal/control"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage/storagetest"
)

const examplePrincipals = "../../examples/real-world-sensor/principals.example.yaml"

func TestPrincipalsApplyProvisionsTheExampleGovernance(t *testing.T) {
	t.Parallel()
	db := filepath.Join(t.TempDir(), "runtime.db")
	steps := []struct {
		args []string
		want string
	}{
		{[]string{"principals", "apply", "--db", db, "--file", examplePrincipals, "--dry-run"}, "dry run, nothing changed): tenant=default active=2 disabled=0 roles=1 memberships=1 authorities=1"},
		{[]string{"principals", "show", "--db", db}, "active=0"},
		{[]string{"principals", "apply", "--db", db, "--file", examplePrincipals}, "principals: tenant=default active=2 disabled=0 roles=1 memberships=1 authorities=1"},
		{[]string{"principals", "show", "--db", db, "--json"}, `"active_principals":2`},
	}
	for _, step := range steps {
		out, err := runOperatorCommand(t, step.args...)
		if err != nil || !strings.Contains(out, step.want) {
			t.Fatalf("%v = %q, %v; want %q", step.args, out, err, step.want)
		}
	}
}

func TestPrincipalsApplyWaitsForTheRuntime(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "runtime.db")
	db, err := storagetest.Open(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	running := &runtimecontrol.RuntimeOwner{DB: db, InstanceID: "serve", Lease: time.Minute}
	if err := running.Claim(t.Context(), "epoch-serve"); err != nil {
		t.Fatal(err)
	}
	if _, err := runOperatorCommand(t, "principals", "apply", "--db", path, "--file", examplePrincipals); err == nil || !strings.Contains(err.Error(), "claim runtime ownership") {
		t.Fatalf("apply while the runtime runs = %v, want an ownership refusal", err)
	}
}
