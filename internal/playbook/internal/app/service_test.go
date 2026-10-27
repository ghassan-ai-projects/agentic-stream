package app

import (
	"context"
	"database/sql"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/playbook/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/playbook/internal/store"
	"github.com/ghassan-ai-projects/agentic-stream/internal/sources"
	"github.com/ghassan-ai-projects/agentic-stream/internal/spec"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage/storagetest"
)

func pumpService(t *testing.T) *Service {
	t.Helper()
	return pumpServiceOn(t, storagetest.OpenTemp(t))
}

func pumpServiceOn(t *testing.T, db *storage.DB) *Service {
	t.Helper()
	compiled, err := spec.CompileFile(t.Context(), "../../../../examples/pump-station/pump-station.situation.yaml")
	if err != nil {
		t.Fatal(err)
	}
	playbook, err := spec.CompilePlaybookFile(compiled, "../../../../examples/pump-station/testdata/site-a.playbook.yaml")
	if err != nil {
		t.Fatal(err)
	}
	queue := func(context.Context, *sql.Tx, domain.AuditRequest) (string, error) { return "sch-audit", nil }
	service, err := New(Config{Store: store.New(db), Spec: compiled, Playbook: playbook, TenantID: "t",
		Clock: sources.NewVirtual(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)), QueueAudit: queue})
	if err != nil {
		t.Fatal(err)
	}
	return service
}

func admission(episodeID string, vibration, axial float64) domain.Admission {
	return domain.Admission{TenantID: "t", EpisodeID: episodeID, SchedulerItemID: "sch-" + episodeID, SituationID: "sit-" + episodeID,
		SituationVersion: 1, Facts: map[string]any{
			"facts.vibration_mean_15m": vibration, "facts.vibration_mean_24h": 3.0, "facts.axial_mean_15m": axial,
			"facts.axial_mean_24h": 2.0, "facts.current_mean_15m": 30.0, "facts.current_mean_24h": 30.0,
		}}
}

func route(t *testing.T, service *Service, admitted domain.Admission) domain.Route {
	t.Helper()
	var routed domain.Route
	if err := service.Store().WithTx(t.Context(), func(tx *store.Tx) error {
		var err error
		routed, err = service.Route(t.Context(), tx, admitted)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	return routed
}

func TestNewRefusesAPlaybookWithoutItsDatabaseSpecOrTenant(t *testing.T) {
	t.Parallel()
	if _, err := New(Config{}); err == nil || !strings.Contains(err.Error(), "are required") {
		t.Fatalf("New without configuration = %v, want it refused", err)
	}
}

func TestRouteSendsEachCaseToItsRuleOrTheModel(t *testing.T) {
	t.Parallel()
	service := pumpService(t)
	if routed := route(t, service, admission("e1", 4.2, 2.0)); routed.Kind != domain.RouteRule || routed.Recommendation.RuleID != "bearing-wear" {
		t.Fatalf("worn pump route = %+v, want the bearing-wear rule", routed)
	}
	if routed := route(t, service, admission("e2", 3.0, 2.6)); routed.Kind != domain.RouteModel {
		t.Fatalf("shadow-only match route = %+v, want the model", routed)
	}
	if service.Digest() == "" {
		t.Fatal("the service names no playbook digest")
	}
}

func TestRouteTripsTheRateCapAfterItsFires(t *testing.T) {
	t.Parallel()
	service := pumpService(t)
	for i := range 4 {
		if routed := route(t, service, admission("e"+string(rune('a'+i)), 4.2, 2.0)); routed.Kind != domain.RouteRule {
			t.Fatalf("fire %d route = %+v, want the rule under its cap", i, routed)
		}
	}
	if routed := route(t, service, admission("e-capped", 4.2, 2.0)); routed.Kind != domain.RouteModel {
		t.Fatalf("capped route = %+v, want the model", routed)
	}
	if routed := route(t, service, admission("e-after", 4.2, 2.0)); routed.Kind != domain.RouteModel {
		t.Fatalf("route after the circuit opened = %+v, want the model", routed)
	}
}

func TestAnAuditItemRoutesAsAnAuditOfItsRule(t *testing.T) {
	t.Parallel()
	service := pumpService(t)
	route(t, service, admission("e1", 4.2, 2.0))
	fire := domain.Fire{FireID: "rfire_e1", TenantID: "t", EpisodeID: "e1", SituationID: "sit-e1", SituationVersion: 1,
		Recommendation: domain.Recommendation{RuleID: "bearing-wear", RuleVersion: 1}}
	if err := service.Store().WithTx(t.Context(), func(tx *store.Tx) error {
		return tx.RecordEvaluation(t.Context(), domain.NewAuditEvaluation(fire, "sch-audit"), time.Now())
	}); err != nil {
		t.Fatal(err)
	}
	audit := admission("e-audit", 4.2, 2.0)
	audit.SchedulerItemID, audit.SituationID = "sch-audit", "sit-e1"
	if routed := route(t, service, audit); routed.Kind != domain.RouteAudit || routed.Recommendation.RuleID != "bearing-wear" {
		t.Fatalf("audit route = %+v, want an audit of bearing-wear", routed)
	}
}

func TestSettleWaitsForEpisodesThatHaveNotEnded(t *testing.T) {
	t.Parallel()
	service := pumpService(t)
	route(t, service, admission("e1", 4.2, 2.0))
	route(t, service, admission("e2", 3.0, 2.6))
	report, err := service.Settle(t.Context())
	if err != nil || report != (domain.Report{}) {
		t.Fatalf("settle = %+v, %v; want nothing settled before the episodes end", report, err)
	}
}

func TestJudgeComparesAnAcceptedDecisionWithTheRecommendation(t *testing.T) {
	t.Parallel()
	service := pumpService(t)
	evaluation := domain.Evaluation{Recommendation: domain.Recommendation{DiagnosisCode: "bearing_wear", Intent: "schedule_maintenance", Preset: "bearing_wear"}}
	if verdict, observed := service.judge(evaluation, store.EpisodeDecision{Ended: true}); verdict != domain.VerdictInconclusive || observed != nil {
		t.Fatalf("judge of no accepted decision = %s %+v, want inconclusive", verdict, observed)
	}
	agreeing := store.EpisodeDecision{Ended: true, Accepted: true, Decision: map[string]any{"diagnosis_code": "bearing_wear", "intents": []any{
		map[string]any{"type": "schedule_maintenance", "parameters": map[string]any{"work_order": "bearing_wear"}}}}}
	if verdict, _ := service.judge(evaluation, agreeing); verdict != domain.VerdictAgree {
		t.Fatalf("judge of the same answer = %s, want agree", verdict)
	}
	if boolCount(true) != 1 || boolCount(false) != 0 {
		t.Fatal("boolCount counts a true as one")
	}
}

func TestARuleOfAKindBornWithAnotherDigestNoLongerAnswers(t *testing.T) {
	t.Parallel()
	service := pumpService(t)
	service.kinds = func() map[string]string { return map[string]string{"pump": "sha256:probe-b"} }
	if routed := route(t, service, admission("e1", 4.2, 2.0)); routed.Kind != domain.RouteModel {
		t.Fatalf("route after a kind change = %+v, want the model", routed)
	}
	if _, err := service.Settle(t.Context()); err != nil {
		t.Fatalf("settle after a kind change: %v", err)
	}
}

func seededService(t *testing.T, extra string) (*Service, *storage.DB) {
	t.Helper()
	db := storagetest.OpenTempWithoutForeignKeys(t)
	fixture, err := os.ReadFile("../../testdata/acted-case.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(t.Context(), string(fixture)+extra); err != nil {
		t.Fatalf("seed the acted case: %v", err)
	}
	service := pumpServiceOn(t, db)
	service.entityKinds = func(string) string { return "sha256:pump" }
	return service, db
}

func caseRow(t *testing.T, db *storage.DB) string {
	t.Helper()
	var row string
	if err := db.QueryRowContext(t.Context(), "SELECT verdict || '/' || kind_digest || '/' || preset FROM case_verifications").Scan(&row); err != nil {
		t.Fatal(err)
	}
	return row
}

func TestSettleVerifiesARuleCaseThatClosedAndStayedClosed(t *testing.T) {
	t.Parallel()
	service, db := seededService(t, "")
	if _, err := service.Settle(t.Context()); err != nil {
		t.Fatal(err)
	}
	if row := caseRow(t, db); row != "verified/sha256:pump/bearing_wear" {
		t.Fatalf("case = %s, want a verified bearing_wear case of the pump kind", row)
	}
}

func TestSettleSwitchesOffARuleWhoseCaseReopened(t *testing.T) {
	t.Parallel()
	reopen := `INSERT INTO situation_versions (situation_id, version, phase, severity, confidence, completeness, event_horizon, valid_from,
		snapshot_json, snapshot_sha256, lineage_id, created_at)
		VALUES ('s1', 3, 'watch', 40, 1, 'on_time', '2026-01-01T12:00:00.000000000Z', 'x', CAST('{}' AS BLOB), zeroblob(32), 'l', 'x');`
	service, db := seededService(t, reopen)
	if _, err := service.Settle(t.Context()); err != nil {
		t.Fatal(err)
	}
	if row := caseRow(t, db); row != "not_verified/sha256:pump/bearing_wear" {
		t.Fatalf("case = %s, want the reopened case not verified", row)
	}
	var cause string
	if err := db.QueryRowContext(t.Context(), "SELECT cause FROM rule_circuits WHERE rule_id = 'bearing-wear'").Scan(&cause); err != nil || cause != "no_effect" {
		t.Fatalf("circuit cause = %q (%v), want no_effect", cause, err)
	}
}
