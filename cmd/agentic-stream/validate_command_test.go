package main

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/policy"
	"github.com/ghassan-ai-projects/agentic-stream/internal/spec"
)

func TestValidatePrintsTheDigestsTheBenchGatewayAllowLists(t *testing.T) {
	t.Parallel()
	compiled, err := spec.CompileFile(t.Context(), experimentSpec)
	if err != nil {
		t.Fatal(err)
	}
	policyDigest, err := policy.DigestForVersion(compiled.Digest)
	if err != nil {
		t.Fatal(err)
	}
	out := runCLI(t, newValidateCommand(), experimentSpec)
	want := "ok: " + compiled.Metadata.Name + "\nversion: " + compiled.Metadata.Version + "\ndigest: " + compiled.Digest + "\npolicy_digest: " + policyDigest + "\nschema: " + compiled.SchemaVersion + "\n"
	if out != want {
		t.Fatalf("validate output =\n%s\nwant\n%s", out, want)
	}
}

func TestValidateJSONPrintsTheCanonicalSpec(t *testing.T) {
	t.Parallel()
	compiled, err := spec.CompileFile(t.Context(), experimentSpec)
	if err != nil {
		t.Fatal(err)
	}
	out := runCLI(t, newValidateCommand(), experimentSpec, "--json")
	if strings.TrimSuffix(out, "\n") != string(compiled.CanonicalJSON) || !json.Valid([]byte(out)) {
		t.Fatalf("validate --json = %.200q, want the canonical JSON of the compiled spec", out)
	}
}

func TestValidateRefusesASpecThatDoesNotCompile(t *testing.T) {
	t.Parallel()
	cmd := newValidateCommand()
	cmd.SetArgs([]string{"missing.situation.yaml"})
	if err := cmd.ExecuteContext(t.Context()); err == nil || !strings.Contains(err.Error(), "compile missing.situation.yaml") {
		t.Fatalf("validate of a missing spec: error = %v, want a compile refusal naming the file", err)
	}
}
