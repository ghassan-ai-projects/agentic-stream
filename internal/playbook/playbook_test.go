package playbook_test

import (
	"database/sql"
	"strings"
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/playbook"
	"github.com/ghassan-ai-projects/agentic-stream/internal/spec"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage/storagetest"
)

func TestTheFacadeRoutesAWornPumpToItsRuleAndSettles(t *testing.T) {
	t.Parallel()
	compiled, err := spec.CompileFile(t.Context(), "../../examples/pump-station/pump-station.situation.yaml")
	if err != nil {
		t.Fatal(err)
	}
	book, err := spec.CompilePlaybookFile(compiled, "../../examples/pump-station/testdata/site-a.playbook.yaml")
	if err != nil {
		t.Fatal(err)
	}
	db := storagetest.OpenTemp(t)
	service, err := playbook.New(playbook.Config{DB: db, Spec: compiled, Playbook: book, TenantID: "t"})
	if err != nil || service.Digest() != book.Digest {
		t.Fatalf("New = %v digest %q, want the playbook's", err, service.Digest())
	}
	admission := playbook.Admission{TenantID: "t", EpisodeID: "e1", SchedulerItemID: "sch-e1", SituationID: "s1", SituationVersion: 1,
		Facts: map[string]any{"facts.vibration_mean_15m": 4.2, "facts.vibration_mean_24h": 3.0, "facts.axial_mean_15m": 2.0,
			"facts.axial_mean_24h": 2.0, "facts.current_mean_15m": 30.0}}
	var route playbook.Route
	if err := db.WithTx(t.Context(), func(tx *sql.Tx) error {
		route, err = service.Route(t.Context(), tx, admission)
		return err
	}); err != nil || route.Kind != playbook.RouteRule {
		t.Fatalf("route = %+v, %v; want the rule", route, err)
	}
	if _, err := service.Settle(t.Context()); err != nil {
		t.Fatalf("settle: %v", err)
	}
	if _, err := playbook.New(playbook.Config{}); err == nil || !strings.Contains(err.Error(), "are required") {
		t.Fatalf("New without configuration = %v, want it refused", err)
	}
}
