package spec_test

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/spec"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
)

func TestFacadeCompilesAndDeploysASpec(t *testing.T) {
	t.Parallel()
	compiled, err := spec.CompileFile(t.Context(), filepath.Join("..", "..", "docs", "design", "examples", "predictive-maintenance.situation.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	db, err := storage.Open(t.Context(), filepath.Join(t.TempDir(), "spec.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	if err := spec.SaveDeployment(t.Context(), db, "tenant", compiled); err != nil {
		t.Fatalf("deploy: %v", err)
	}
	if err := spec.SaveDeployment(t.Context(), db, "tenant", compiled); err != nil {
		t.Fatalf("redeploy of the same digest must be idempotent: %v", err)
	}
}

func TestFacadeDurationsAndEventSchemas(t *testing.T) {
	t.Parallel()
	if got, err := spec.ParseDuration("2d"); err != nil || got != 48*time.Hour {
		t.Fatalf("2d = %v, %v", got, err)
	}
	if _, err := spec.ParseDuration("0d"); err == nil {
		t.Fatal("zero days accepted")
	}
	if _, ok := spec.LookupEventSchema("motor.vibration.observed/1.0"); !ok {
		t.Fatal("built-in event schema missing")
	}
	if _, err := spec.NewCELEnv(); err != nil {
		t.Fatal(err)
	}
}

func TestLoadDeploymentReturnsTheDeployedSpec(t *testing.T) {
	t.Parallel()
	compiled, err := spec.CompileFile(t.Context(), filepath.Join("..", "..", "docs", "design", "examples", "predictive-maintenance.situation.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	db, err := storage.Open(t.Context(), filepath.Join(t.TempDir(), "spec.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	if err := spec.SaveDeployment(t.Context(), db, "tenant", compiled); err != nil {
		t.Fatal(err)
	}
	loaded, err := spec.LoadDeployment(t.Context(), db, compiled.Digest)
	if err != nil || loaded.Metadata.Name != compiled.Metadata.Name || len(loaded.FieldDerivations()) != len(compiled.FieldDerivations()) {
		t.Fatalf("loaded = %+v, %v", loaded, err)
	}
	if _, err := spec.LoadDeployment(t.Context(), db, "sha256:missing"); err == nil {
		t.Fatal("an undeployed spec was loaded")
	}
}

func TestCELBoolAcceptsOnlyBooleans(t *testing.T) {
	t.Parallel()
	env, err := spec.NewCELEnv()
	if err != nil {
		t.Fatal(err)
	}
	evaluate := func(expression string) (bool, error) {
		ast, issues := env.Compile(expression)
		if issues != nil && issues.Err() != nil {
			t.Fatal(issues.Err())
		}
		program, err := env.Program(ast)
		if err != nil {
			t.Fatal(err)
		}
		out, _, err := program.Eval(map[string]any{})
		if err != nil {
			t.Fatal(err)
		}
		return spec.CELBool(out)
	}
	if got, err := evaluate("1 < 2"); err != nil || !got {
		t.Fatalf("1 < 2 = %t, %v", got, err)
	}
	if _, err := evaluate("1 + 2"); err == nil {
		t.Fatal("a non-boolean result was accepted")
	}
}
