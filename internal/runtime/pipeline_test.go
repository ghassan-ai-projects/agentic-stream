package runtime_test

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	runtimecontrol "github.com/ghassan-ai-projects/agentic-stream/internal/control"
	"github.com/ghassan-ai-projects/agentic-stream/internal/policy"
	"github.com/ghassan-ai-projects/agentic-stream/internal/runtime"
	"github.com/ghassan-ai-projects/agentic-stream/internal/spec"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage/storagetest"
	"github.com/ghassan-ai-projects/agentic-stream/internal/testsupport/workerfake"
)

const (
	examplePolicy = "../../examples/predictive-maintenance/predictive-maintenance.situation.yaml"
	exampleTrace  = "../../examples/predictive-maintenance/testdata/trace-opening.jsonl"
)

func TestNewPipelineRequiresItsDatabaseSpecAndAConsistentOwner(t *testing.T) {
	t.Parallel()
	db := storagetest.OpenTemp(t)
	compiled, err := spec.CompileFile(t.Context(), examplePolicy)
	if err != nil {
		t.Fatal(err)
	}
	owner := &runtimecontrol.RuntimeOwner{DB: db, InstanceID: "instance"}
	tests := map[string]runtime.PipelineConfig{
		"empty":                  {},
		"no spec":                {DB: db},
		"no database":            {Spec: compiled},
		"an owner without epoch": {DB: db, Spec: compiled, Owner: owner},
		"an epoch without owner": {DB: db, Spec: compiled, OwnerEpoch: "epoch"},
	}
	for name, cfg := range tests {
		if pipeline, err := runtime.NewPipeline(t.Context(), cfg); err == nil || pipeline != nil {
			t.Errorf("%s: NewPipeline = (%v, %v), want a refusal", name, pipeline, err)
		}
	}
}

func TestAnAbsentPipelineStartsNothingAndClosesCleanly(t *testing.T) {
	t.Parallel()
	var pipeline *runtime.Pipeline
	if err := pipeline.Start(t.Context()); err == nil {
		t.Fatal("nil pipeline started")
	}
	if err := pipeline.Close(); err != nil {
		t.Fatalf("nil pipeline close: %v", err)
	}
}

func TestPipelineFacadeDelegatesEveryOperationToTheUseCases(t *testing.T) {
	t.Parallel()
	compiled, err := spec.CompileFile(t.Context(), examplePolicy)
	if err != nil {
		t.Fatal(err)
	}
	pipeline, err := runtime.NewPipeline(t.Context(), runtime.PipelineConfig{DB: storagetest.OpenTemp(t), Spec: compiled})
	if err != nil {
		t.Fatal(err)
	}
	if err := pipeline.Start(t.Context()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = pipeline.Close() })

	if report, err := pipeline.RunJSONL(t.Context(), exampleTrace); err != nil || report.EventsIngested != 1 || report.EventsProcessed != 1 {
		t.Fatalf("RunJSONL = %+v, %v; want one event ingested and processed", report, err)
	}
	if _, err := pipeline.RunSimulatorJSONL(t.Context(), filepath.Join(t.TempDir(), "missing.jsonl")); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("RunSimulatorJSONL over a missing trace = %v, want not exist", err)
	}
	occupied := filepath.Join(workerfake.SocketDir(t), "occupied")
	if err := os.WriteFile(occupied, []byte("not a socket"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := pipeline.RunLiveSocket(t.Context(), occupied); err == nil || !strings.Contains(err.Error(), "refusing unsafe existing live socket path") {
		t.Fatalf("RunLiveSocket over an existing file = %v, want it refused", err)
	}
	if pipeline.AdvanceEvery(t.Context(), 0) == nil || pipeline.RunEpisodesEvery(t.Context(), 0) == nil {
		t.Fatal("a schedule without a positive interval was accepted")
	}
	_, presentErr := pipeline.ApprovalForSigning(t.Context(), policy.ApprovalLookup{ID: "absent"})
	_, resolveErr := pipeline.ResolveApproval(t.Context(), policy.ApprovalResolution{ID: "absent"})
	if !errors.Is(presentErr, policy.ErrApprovalNotFound) || !errors.Is(resolveErr, policy.ErrApprovalNotFound) {
		t.Fatalf("approvals for an absent request: present=%v resolve=%v; want not found", presentErr, resolveErr)
	}
}
