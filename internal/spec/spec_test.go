package spec_test

import (
	"database/sql"
	"errors"
	"io/fs"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/spec"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage/storagetest"
)

var predictiveMaintenance = filepath.Join("..", "..", "docs", "design", "examples", "predictive-maintenance.situation.yaml")

func TestFacadeCompilesDeploysAndLoadsASpec(t *testing.T) {
	t.Parallel()
	compiled, err := spec.CompileFile(t.Context(), predictiveMaintenance)
	if err != nil {
		t.Fatal(err)
	}
	db := storagetest.OpenTemp(t)

	for range 2 {
		if err := spec.SaveDeployment(t.Context(), db, "tenant", compiled); err != nil {
			t.Fatalf("deploy (a redeploy of the same digest is idempotent): %v", err)
		}
	}

	loaded, err := spec.LoadDeployment(t.Context(), db, compiled.Digest)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Digest != compiled.Digest || len(loaded.FieldDerivations()) != len(compiled.FieldDerivations()) {
		t.Fatalf("loaded %s with %d derivations, want %s with %d", loaded.Digest, len(loaded.FieldDerivations()), compiled.Digest, len(compiled.FieldDerivations()))
	}
}

func TestFacadeNamesTheFileAndWrapsTheCauseWhenCompilationFails(t *testing.T) {
	t.Parallel()
	missing := filepath.Join(t.TempDir(), "missing.yaml")

	_, err := spec.CompileFile(t.Context(), missing)

	if !errors.Is(err, fs.ErrNotExist) || !strings.Contains(err.Error(), "compile spec "+missing) {
		t.Fatalf("err = %v, want fs.ErrNotExist naming %s", err, missing)
	}
}

func TestFacadeRefusesToDeployWhatItCannotIdentify(t *testing.T) {
	t.Parallel()
	db := storagetest.OpenTemp(t)

	err := spec.SaveDeployment(t.Context(), db, "tenant", nil)

	if err == nil || !strings.Contains(err.Error(), "save deployment: ") || !strings.Contains(err.Error(), "compiled spec is nil") {
		t.Fatalf("err = %v, want the nil spec refusal wrapped by save deployment", err)
	}
}

func TestFacadeReportsAnUndeployedSpec(t *testing.T) {
	t.Parallel()
	db := storagetest.OpenTemp(t)

	_, err := spec.LoadDeployment(t.Context(), db, "sha256:missing")

	if !errors.Is(err, sql.ErrNoRows) || !strings.Contains(err.Error(), "load deployment: ") {
		t.Fatalf("err = %v, want sql.ErrNoRows wrapped by load deployment", err)
	}
}

func TestFacadeExposesTheSpecVocabulary(t *testing.T) {
	t.Parallel()
	if got, err := spec.ParseDuration("2d"); err != nil || got != 48*time.Hour {
		t.Fatalf("ParseDuration(2d) = %v, %v, want 48h", got, err)
	}
	if schema, ok := spec.LookupEventSchema("motor.vibration.observed/1.0"); !ok || schema.EventType != "motor.vibration.observed" {
		t.Fatalf("LookupEventSchema = %+v, %t, want the built-in vibration schema", schema, ok)
	}
	if got := spec.EffectiveDispatchPolicy(""); got != spec.DispatchShadow {
		t.Fatalf("EffectiveDispatchPolicy(\"\") = %q, want %q", got, spec.DispatchShadow)
	}
	if _, err := spec.NewCELEnv(); err != nil {
		t.Fatal(err)
	}
}
