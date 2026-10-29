package store

import (
	"strings"
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/canonicaljson"
	"github.com/ghassan-ai-projects/agentic-stream/internal/episodes"
	"github.com/ghassan-ai-projects/agentic-stream/internal/playbook"
	"github.com/ghassan-ai-projects/agentic-stream/internal/spec"
)

func pumpPlaybook(t *testing.T, store *PipelineStore) {
	t.Helper()
	compiled, err := spec.CompileFile(t.Context(), "../../../../examples/pump-station/pump-station.situation.yaml")
	if err != nil {
		t.Fatal(err)
	}
	book, err := spec.CompilePlaybookFile(compiled, "../../../../examples/pump-station/testdata/site-a.playbook.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if store.Playbook, err = playbook.New(playbook.Config{DB: store.DB, Spec: compiled, Playbook: book, TenantID: store.TenantID}); err != nil {
		t.Fatal(err)
	}
}

func wornPumpRequest(t *testing.T) *episodes.Request {
	t.Helper()
	raw, err := canonicaljson.Marshal(map[string]any{"snapshot": map[string]any{"facts": map[string]any{
		"facts.vibration_mean_15m": 4.2, "facts.vibration_mean_24h": 3.0, "facts.axial_mean_15m": 2.0,
		"facts.axial_mean_24h": 2.0, "facts.current_mean_15m": 30.0}, "event_horizon": "2026-01-01T00:00:00.000000000Z"}})
	if err != nil {
		t.Fatal(err)
	}
	return &episodes.Request{EpisodeID: "ep-1", SchedulerItemID: "item-1", SituationID: "situation-1", SituationVersion: 1, RequestJSON: raw}
}

func routeIn(t *testing.T, store *PipelineStore, req *episodes.Request) (playbook.Route, error) {
	t.Helper()
	var route playbook.Route
	err := store.InAdmission(t.Context(), func(tx *AdmissionTx) error {
		var err error
		route, err = tx.Route(t.Context(), req)
		if tx.PlaybookDigest() == "" && store.Playbook != nil {
			t.Fatal("the admission names no playbook digest")
		}
		return err
	})
	return route, err
}

func TestAdmissionRoutesThroughThePlaybookWhenOneIsLoaded(t *testing.T) {
	t.Parallel()
	_, store := seedPendingItem(t, "tenant")
	if route, err := routeIn(t, store, wornPumpRequest(t)); err != nil || route.Kind != playbook.RouteModel {
		t.Fatalf("route without a playbook = %+v, %v; want the model", route, err)
	}
	if err := store.SettlePlaybook(t.Context()); err != nil {
		t.Fatalf("settle without a playbook: %v", err)
	}
	pumpPlaybook(t, store)
	if route, err := routeIn(t, store, wornPumpRequest(t)); err != nil || route.Kind != playbook.RouteRule {
		t.Fatalf("route of a worn pump = %+v, %v; want its rule", route, err)
	}
	if err := store.SettlePlaybook(t.Context()); err != nil {
		t.Fatalf("settle: %v", err)
	}
	broken := wornPumpRequest(t)
	broken.RequestJSON = []byte("{")
	if _, err := routeIn(t, store, broken); err == nil || !strings.Contains(err.Error(), "read the snapshot to route episode ep-1") {
		t.Fatalf("route of an unreadable request = %v, want it refused", err)
	}
}
