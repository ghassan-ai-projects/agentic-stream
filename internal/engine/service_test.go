package engine_test

import (
	"strings"
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/contractsv1"
	"github.com/ghassan-ai-projects/agentic-stream/internal/engine"
	"github.com/ghassan-ai-projects/agentic-stream/internal/eventlog"
	"github.com/ghassan-ai-projects/agentic-stream/internal/spec"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage/storagetest"
)

const exampleSpec = "../../examples/predictive-maintenance/predictive-maintenance.situation.yaml"

func completeConfig(t *testing.T) (engine.Config, *storage.DB) {
	t.Helper()
	db := storagetest.OpenTemp(t)
	compiled, err := spec.CompileFile(t.Context(), exampleSpec)
	if err != nil {
		t.Fatal(err)
	}
	return engine.Config{DB: db, Log: eventlog.NewEventLog(db), Spec: compiled, RuntimeOwner: engine.ReplayOwnership, Epoch: "epoch"}, db
}

func TestNewRefusesEveryMissingSafetyDependency(t *testing.T) {
	t.Parallel()
	removals := map[string]struct {
		remove func(*engine.Config)
		want   string
	}{
		"database": {func(c *engine.Config) { c.DB = nil }, "engine requires a database"},
		"log":      {func(c *engine.Config) { c.Log = nil }, "engine requires a database"},
		"spec":     {func(c *engine.Config) { c.Spec = nil }, "engine requires a compiled spec"},
		"owner":    {func(c *engine.Config) { c.RuntimeOwner = nil }, "engine requires a database"},
	}
	for name, tc := range removals {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			cfg, _ := completeConfig(t)
			tc.remove(&cfg)
			service, err := engine.New(t.Context(), cfg)
			if err == nil || service != nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("engine without %s: service=%v err=%v, want %q", name, service, err, tc.want)
			}
		})
	}
}

func TestEngineAppliesEvidenceAndExposesItsSituationsThroughTheFacade(t *testing.T) {
	t.Parallel()
	cfg, db := completeConfig(t)
	service, err := engine.New(t.Context(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	if processed, err := service.RunGlobal(t.Context(), nil); err != nil || processed != 0 {
		t.Fatalf("empty log processed=%d err=%v, want 0", processed, err)
	}
	for i, rms := range []float64{9, 10, 11, 12} {
		when := time.Date(2026, 1, 1, 0, i, 0, 0, time.UTC)
		event := contractsv1.Envelope{ID: "evt-" + string(rune('0'+i)), Type: "motor.vibration.observed", SchemaVersion: "1.0", TenantID: contractsv1.TenantID,
			Source: "simulator", PartitionKey: "motor-17", Entity: contractsv1.EntityRef{Type: "motor", ID: "motor-17"}, EventTime: when, IngestedAt: when,
			Classification: contractsv1.ClassificationInternal, Data: map[string]any{"rms_mm_s": rms}}
		if _, err := cfg.Log.Append(t.Context(), contractsv1.TenantID, []contractsv1.Envelope{event}); err != nil {
			t.Fatal(err)
		}
	}
	if processed, err := service.RunGlobal(t.Context(), nil); err != nil || processed != 4 {
		t.Fatalf("processed=%d err=%v, want the four events of the default tenant", processed, err)
	}

	listed, err := engine.ListSituations(t.Context(), db, contractsv1.TenantID, "motor-17")
	if err != nil || len(listed) != 1 || listed[0].SituationType != "bearing_degradation" || listed[0].EntityID != "motor-17" {
		t.Fatalf("listed = %+v err=%v, want one bearing_degradation situation of motor-17", listed, err)
	}
	version, err := engine.SituationVersion(t.Context(), db, contractsv1.TenantID, listed[0].SituationID, 0)
	if err != nil || version.Version != listed[0].CurrentVersion || version.SituationID != listed[0].SituationID {
		t.Fatalf("version = %+v err=%v, want the current version of the listed situation", version, err)
	}
	if _, err := engine.SituationVersion(t.Context(), db, contractsv1.TenantID, "sit-missing", 0); err == nil {
		t.Fatal("an unknown situation was read")
	}
}

func TestReplayOwnershipAssertsNothing(t *testing.T) {
	t.Parallel()
	if err := engine.ReplayOwnership(t.Context(), nil, "any-epoch"); err != nil {
		t.Fatalf("err = %v, want none: replay has no runtime owner", err)
	}
}

func TestEngineUsesTheDefaultTenantUnlessAnotherIsConfigured(t *testing.T) {
	t.Parallel()
	cfg, _ := completeConfig(t)
	cfg.TenantID = "tenant-b"
	service, err := engine.New(t.Context(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	event := contractsv1.Envelope{ID: "evt-default", Type: "motor.vibration.observed", SchemaVersion: "1.0", TenantID: contractsv1.TenantID,
		Source: "simulator", PartitionKey: "motor-17", Entity: contractsv1.EntityRef{Type: "motor", ID: "motor-17"},
		EventTime: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), IngestedAt: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		Classification: contractsv1.ClassificationInternal, Data: map[string]any{"rms_mm_s": 12.0}}
	if _, err := cfg.Log.Append(t.Context(), contractsv1.TenantID, []contractsv1.Envelope{event}); err != nil {
		t.Fatal(err)
	}
	if processed, err := service.RunGlobal(t.Context(), nil); err != nil || processed != 0 {
		t.Fatalf("processed=%d err=%v, want 0: an engine of tenant-b must not apply the default tenant's evidence", processed, err)
	}
}
