package main

import (
	"encoding/json"
	"os"
	"path/filepath"
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

const fanSpeedSchema = `{"fan.speed.observed/1.0": {"event_type": "fan.speed.observed", "schema_version": "1.0", "fields": {"rpm": {"path": "rpm", "unit": "rpm"}}}}`

func eventSchemaDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "fan.json"), []byte(fanSpeedSchema), 0o600); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestValidateWithEventSchemasPrintsTheSameDigestsForEveryExampleSpec(t *testing.T) {
	t.Parallel()
	specs, err := filepath.Glob("../../examples/*/*.situation.yaml")
	if err != nil || len(specs) == 0 {
		t.Fatalf("find example specs: %v (%d found)", err, len(specs))
	}
	dir := eventSchemaDir(t)
	for _, path := range specs {
		embedded := runCLI(t, newValidateCommand(), path)
		withDirectory := runCLI(t, newRootCommand(), "validate", path, "--event-schemas", dir)
		if withDirectory != embedded {
			t.Fatalf("%s: validate --event-schemas =\n%s\nwant the embedded result\n%s", path, withDirectory, embedded)
		}
	}
}

func TestValidateWithEventSchemasCompilesASpecThatUsesADirectorySchema(t *testing.T) {
	t.Parallel()
	source, err := os.ReadFile(experimentSpec)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	if err := writeInto(t, dir, "fan.situation.yaml", strings.ReplaceAll(string(source), "zone.heartbeat.observed", "fan.speed.observed")); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "fan.situation.yaml")
	cmd := newValidateCommand()
	cmd.SetArgs([]string{path})
	if err := cmd.ExecuteContext(t.Context()); err == nil || !strings.Contains(err.Error(), `unknown event schema "fan.speed.observed/1.0"`) {
		t.Fatalf("validate without the directory = %v, want the fan schema unknown", err)
	}
	if out := runCLI(t, newRootCommand(), "validate", path, "--event-schemas", eventSchemaDir(t)); !strings.HasPrefix(out, "ok: ") {
		t.Fatalf("validate --event-schemas = %q, want ok", out)
	}
}

func TestValidateRefusesAnEventSchemaDirectoryItCannotRead(t *testing.T) {
	t.Parallel()
	cmd := newRootCommand()
	cmd.SetArgs([]string{"validate", experimentSpec, "--event-schemas", filepath.Join(t.TempDir(), "absent")})
	if err := cmd.ExecuteContext(t.Context()); err == nil || !strings.Contains(err.Error(), "--event-schemas: load event schemas from") {
		t.Fatalf("validate with a missing schema directory = %v, want it refused", err)
	}
}

func writeInto(t *testing.T, dir, name, content string) error {
	t.Helper()
	root, err := os.OpenRoot(dir)
	if err != nil {
		return err
	}
	defer func() { _ = root.Close() }()
	return root.WriteFile(name, []byte(content), 0o600)
}

func TestValidateWithAPlaybookPrintsItsDigestAndKeepsThePolicyDigest(t *testing.T) {
	t.Parallel()
	const path = "../../examples/pump-station/testdata/site-a.playbook.yaml"
	playbook, err := spec.CompilePlaybookFile(compilePumpStation(t), path)
	if err != nil {
		t.Fatal(err)
	}
	plain := runCLI(t, newValidateCommand(), pumpStationSpec)
	out := runCLI(t, newValidateCommand(), pumpStationSpec, "--playbook", path)
	want := plain + "playbook: site-a\nplaybook_rules: 2\nplaybook_digest: " + playbook.Digest + "\n"
	if out != want {
		t.Fatalf("validate --playbook =\n%s\nwant\n%s", out, want)
	}
}
