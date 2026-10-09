package runtime_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/executor/fixture"
	"github.com/ghassan-ai-projects/agentic-stream/internal/runtime"
	"github.com/ghassan-ai-projects/agentic-stream/internal/sources"
	"github.com/ghassan-ai-projects/agentic-stream/internal/spec"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage/storagetest"
)

const acceptanceTrace = "../../examples/predictive-maintenance/testdata/trace-acceptance.jsonl"

type acceptanceRun struct {
	t        *testing.T
	db       *storage.DB
	clock    *sources.Virtual
	pipeline *runtime.Pipeline
}

func runPredictiveMaintenanceAcceptance(t *testing.T, dispatchPolicy string) acceptanceRun {
	t.Helper()
	compiled := compilePredictiveMaintenance(t, dispatchPolicy)
	run := acceptanceRun{t: t, db: storagetest.OpenTemp(t), clock: sources.NewVirtual(time.Date(2026, 1, 1, 1, 0, 0, 0, time.UTC))}
	var err error
	run.pipeline, err = runtime.NewPipeline(t.Context(), runtime.PipelineConfig{
		DB: run.db, Spec: compiled, TenantID: "default", Clock: run.clock, IDGenerator: sources.Deterministic(),
		Executor: fixture.New(), DemoMode: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = run.pipeline.Close() })
	if _, err := run.pipeline.RunJSONL(t.Context(), acceptanceTrace); err != nil {
		t.Fatalf("run the acceptance trace: %v", err)
	}
	return run
}

func compilePredictiveMaintenance(t *testing.T, dispatchPolicy string) *spec.CompiledSpec {
	t.Helper()
	source, err := os.ReadFile(examplePolicy)
	if err != nil {
		t.Fatal(err)
	}
	const executor = "  executor:\n    name: native\n"
	if !strings.Contains(string(source), executor) {
		t.Fatal("the predictive-maintenance executor block moved")
	}
	path := filepath.Join(t.TempDir(), "predictive-maintenance.situation.yaml")
	edited := strings.Replace(string(source), executor, executor+"    dispatchPolicy: "+dispatchPolicy+"\n", 1)
	if err := os.WriteFile(path, []byte(edited), 0o600); err != nil { //nolint:gosec // The path is the test's own temporary directory.
		t.Fatal(err)
	}
	compiled, err := spec.CompileFile(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}
	return compiled
}

func (r acceptanceRun) text(query string) string {
	r.t.Helper()
	var value string
	if err := r.db.QueryRowContext(r.t.Context(), query).Scan(&value); err != nil {
		r.t.Fatalf("%s: %v", query, err)
	}
	return value
}

func (r acceptanceRun) advanceUntil(what string, done func() bool) {
	r.t.Helper()
	ctx, stop := context.WithCancel(r.t.Context())
	defer stop()
	loop := make(chan error, 1)
	go func() { loop <- r.pipeline.AdvanceEvery(ctx, time.Millisecond) }()
	deadline := time.Now().Add(10 * time.Second)
	for !done() {
		if time.Now().After(deadline) {
			r.t.Fatalf("timed out waiting for %s: items=%s episodes=%s decisions=%s intents=%s", what,
				r.text("SELECT COALESCE(group_concat(status || '@' || COALESCE(not_before,'-') || '<' || expires_at), '') FROM scheduler_items"),
				r.text("SELECT COALESCE(group_concat(lifecycle_status), '') FROM episodes"),
				r.text("SELECT COALESCE(group_concat(validation_status), '') FROM decisions"),
				r.text("SELECT COALESCE(group_concat(policy_status), '') FROM intents"))
		}
		select {
		case err := <-loop:
			r.t.Fatalf("advance loop stopped before %s: %v", what, err)
		case <-time.After(5 * time.Millisecond):
		}
	}
	stop()
	<-loop
}

func TestPredictiveMaintenanceAcceptance(t *testing.T) {
	t.Parallel()
	run := runPredictiveMaintenanceAcceptance(t, "active")
	if got := run.text("SELECT COUNT(*) FROM event_log WHERE event_id = 'pm-acc-vib-005'"); got != "1" {
		t.Fatalf("duplicate delivery: pm-acc-vib-005 logged %s times, want once", got)
	}
	if got := run.text("SELECT COUNT(*) FROM event_time_dispositions WHERE event_id = 'pm-acc-vib-021'"); got != "0" {
		t.Fatal("out-of-order evidence within the allowance was treated as late")
	}
	got := run.text("SELECT group_concat(event_id || ':' || disposition, ' ') FROM (SELECT * FROM event_time_dispositions ORDER BY event_id)")
	if got != "pm-acc-late-beyond:beyond_allowed_lateness pm-acc-late-within:corrected" {
		t.Fatalf("late-data dispositions = %q, want one correction and one refusal", got)
	}
	if got := run.text("SELECT group_concat(phase, '>') FROM (SELECT DISTINCT phase FROM situation_versions ORDER BY version)"); got != "candidate>watch>warning" {
		t.Fatalf("hysteresis: phases = %q, want candidate>watch>warning without flapping on the dip", got)
	}
	if got := run.text("SELECT COUNT(*) FROM situation_versions v WHERE NOT EXISTS (SELECT 1 FROM trigger_evaluations t WHERE t.situation_id = v.situation_id AND t.situation_version = v.version)"); got != "0" {
		t.Fatalf("explainability: %s versions have no trigger evaluation", got)
	}
	run.clock.Advance(3 * time.Minute)
	run.advanceUntil("the maintenance ticket command", func() bool {
		return run.text("SELECT COUNT(*) FROM commands WHERE status = 'succeeded'") != "0"
	})
	if got := run.text("SELECT group_concat(intent_type || ':' || policy_status) FROM intents"); got != "create_maintenance_ticket:approved" {
		t.Fatalf("intents = %q, want one approved maintenance ticket", got)
	}
	if got := run.text("SELECT COUNT(*) || '/' || COUNT(DISTINCT command_id) FROM outcomes"); got != "1/1" {
		t.Fatalf("outcomes/commands = %s, want one outcome per command", got)
	}
	run.clock.Advance(10 * time.Minute)
	run.advanceUntil("the missing heartbeat", func() bool {
		return run.text("SELECT completeness FROM situation_versions ORDER BY version DESC LIMIT 1") == "uncertain"
	})
}

func TestPredictiveMaintenanceInShadowModeRecordsComparisonsWithoutEffects(t *testing.T) {
	t.Parallel()
	run := runPredictiveMaintenanceAcceptance(t, "shadow")
	run.clock.Advance(3 * time.Minute)
	run.advanceUntil("the shadow decision", func() bool {
		return run.text("SELECT COUNT(*) FROM shadow_decisions") != "0"
	})
	if got := run.text("SELECT (SELECT COUNT(*) FROM intents) || '/' || (SELECT COUNT(*) FROM commands) || '/' || (SELECT COUNT(*) FROM outbox)"); got != "0/0/0" {
		t.Fatalf("intents/commands/outbox = %s in shadow mode, want none", got)
	}
}
