package spec_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/spec"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage/storagetest"
)

const pulseSchema = `{"motor.pulse.observed/1.0": {"event_type": "motor.pulse.observed", "schema_version": "1.0", "fields": {}}}`

func pulseSpecAndSchemas(t *testing.T) (string, string) {
	t.Helper()
	source, err := os.ReadFile(predictiveMaintenance)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	root, err := os.OpenRoot(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = root.Close() }()
	if err := root.WriteFile("pulse.situation.yaml", []byte(strings.ReplaceAll(string(source), "motor.heartbeat.observed", "motor.pulse.observed")), 0o600); err != nil {
		t.Fatal(err)
	}
	specPath := filepath.Join(dir, "pulse.situation.yaml")
	schemaDir := filepath.Join(dir, "schemas")
	if err := os.Mkdir(schemaDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(schemaDir, "pulse.json"), []byte(pulseSchema), 0o600); err != nil {
		t.Fatal(err)
	}
	return specPath, schemaDir
}

func TestASpecCompilesAndDeploysAgainstSchemasLoadedFromADirectory(t *testing.T) {
	t.Parallel()
	specPath, schemaDir := pulseSpecAndSchemas(t)
	if _, err := spec.CompileFile(t.Context(), specPath); err == nil || !strings.Contains(err.Error(), `unknown event schema "motor.pulse.observed/1.0"`) {
		t.Fatalf("compile against the built-ins = %v, want the pulse schema unknown", err)
	}
	schemas, err := spec.LoadEventSchemas(schemaDir)
	if err != nil {
		t.Fatal(err)
	}
	compiled, err := spec.CompileFile(t.Context(), specPath, spec.WithEventSchemas(schemas))
	if err != nil {
		t.Fatal(err)
	}
	db := storagetest.OpenTemp(t)
	if err := spec.SaveDeployment(t.Context(), db, "tenant", compiled); err != nil {
		t.Fatal(err)
	}
	var registered int
	if err := db.QueryRowContext(t.Context(), "SELECT COUNT(*) FROM event_schemas WHERE schema_id = 'motor.pulse.observed/1.0'").Scan(&registered); err != nil {
		t.Fatal(err)
	}
	if registered != 1 {
		t.Fatalf("registered pulse schemas = %d, want 1", registered)
	}
}

func TestTheBuiltInRegistryCompilesTheSameDigestAsNoRegistry(t *testing.T) {
	t.Parallel()
	plain, err := spec.CompileFile(t.Context(), predictiveMaintenance)
	if err != nil {
		t.Fatal(err)
	}
	_, schemaDir := pulseSpecAndSchemas(t)
	schemas, err := spec.LoadEventSchemas(schemaDir)
	if err != nil {
		t.Fatal(err)
	}
	withDirectory, err := spec.CompileFile(t.Context(), predictiveMaintenance, spec.WithEventSchemas(schemas))
	if err != nil {
		t.Fatal(err)
	}
	if plain.Digest != withDirectory.Digest {
		t.Fatalf("a directory that only adds schemas changed the spec digest: %s, want %s", withDirectory.Digest, plain.Digest)
	}
}

func TestLoadEventSchemasNamesTheDirectory(t *testing.T) {
	t.Parallel()
	missing := filepath.Join(t.TempDir(), "absent")
	if _, err := spec.LoadEventSchemas(missing); err == nil || !strings.Contains(err.Error(), "load event schemas from "+missing) {
		t.Fatalf("LoadEventSchemas = %v, want the directory named", err)
	}
}

func TestTheEventSchemaDigestNamesExactlyTheRegisteredSchemas(t *testing.T) {
	t.Parallel()
	builtin, err := spec.BuiltinEventSchemas().Digest()
	if err != nil || !strings.HasPrefix(builtin, "sha256:") {
		t.Fatalf("built-in digest = %q, %v", builtin, err)
	}
	var unset *spec.EventSchemas
	if fallback, err := unset.Digest(); err != nil || fallback != builtin {
		t.Fatalf("a nil registry digest = %q, %v, want the built-in %q", fallback, err, builtin)
	}
	_, schemaDir := pulseSpecAndSchemas(t)
	schemas, err := spec.LoadEventSchemas(schemaDir)
	if err != nil {
		t.Fatal(err)
	}
	if extended, err := schemas.Digest(); err != nil || extended == builtin {
		t.Fatalf("a registry with one more schema digests to %q, %v; want a digest other than the built-in", extended, err)
	}
}
