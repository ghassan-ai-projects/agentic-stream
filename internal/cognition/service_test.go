package cognition_test

import (
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/cognition"
	"github.com/ghassan-ai-projects/agentic-stream/internal/situations"
	"github.com/ghassan-ai-projects/agentic-stream/internal/sources"
	"github.com/ghassan-ai-projects/agentic-stream/internal/spec"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage/storagetest"
)

const digest = "sha256:0000000000000000000000000000000000000000000000000000000000000000"

func newService(t *testing.T) *cognition.Service {
	t.Helper()
	compiled := &spec.CompiledSpec{
		Digest: digest,
		Cognition: spec.Cognition{Triggers: []spec.Trigger{
			{Name: "high", When: "true", Score: "situation.severity", Threshold: 5, Lane: "fast"},
		}},
	}
	service, err := cognition.New(cognition.Config{DeploymentID: "dep", TenantID: "tenant", Spec: compiled, Clock: sources.NewVirtual(time.Date(2026, 10, 8, 12, 0, 0, 0, time.UTC))})
	if err != nil {
		t.Fatal(err)
	}
	return service
}

func processed(t *testing.T) (*storage.DB, *cognition.Service) {
	t.Helper()
	db := storagetest.OpenTempWithoutForeignKeys(t)
	service := newService(t)
	if _, err := db.ExecContext(t.Context(), `INSERT INTO situations (
		situation_id, tenant_id, deployment_id, situation_type, entity_type, entity_id, partition_id, occurrence_id,
		current_version, last_reasoned_version, phase, status, first_event_time, latest_event_time, updated_at, created_at
	) VALUES ('sit-1', 'tenant', 'dep', 'test', 'motor', 'm1', 0, 'occ', 1, 0, 'watch', 'active', 'now', 'now', 'now', 'now')`); err != nil {
		t.Fatal(err)
	}
	v := situations.Version{SituationID: "sit-1", Version: 1, Phase: "watch", Severity: 30, Confidence: 1}
	if err := db.WithTx(t.Context(), func(tx *sql.Tx) error { return service.Process(t.Context(), tx, v) }); err != nil {
		t.Fatal(err)
	}
	return db, service
}

func TestConfiguredFacadeRefusesMissingConfigurationAndTransaction(t *testing.T) {
	t.Parallel()
	if _, err := cognition.New(cognition.Config{}); err == nil {
		t.Fatal("a missing spec, deployment and tenant were accepted")
	}
	service := newService(t)
	if err := service.Process(t.Context(), nil, situations.Version{}); err == nil {
		t.Fatal("a missing caller transaction was accepted")
	}
	if err := cognition.RecordCostRejectionReason(t.Context(), nil, "item", errors.New("budget")); err == nil {
		t.Fatal("a cost refusal without a transaction was accepted")
	}
	if err := cognition.RecordSchedulerExpiryReason(t.Context(), nil, "item", "expired"); err == nil {
		t.Fatal("a scheduler expiry without a transaction was accepted")
	}
}

func TestProcessedVersionIsReadableThroughTheFacadeReaders(t *testing.T) {
	t.Parallel()
	db, _ := processed(t)
	listed, err := cognition.TriggerEvaluations(t.Context(), db.DB, "tenant", "sit-1", 1)
	if err != nil || len(listed) != 1 || listed[0].Outcome != "admitted" || listed[0].TriggerName != "high" {
		t.Fatalf("TriggerEvaluations = %+v, %v", listed, err)
	}
	one, err := cognition.TriggerEvaluation(t.Context(), db.DB, "tenant", listed[0].TriggerID)
	if err != nil || one.TriggerID != listed[0].TriggerID {
		t.Fatalf("TriggerEvaluation = %+v, %v", one, err)
	}
	if _, err := cognition.TriggerEvaluation(t.Context(), db.DB, "other-tenant", listed[0].TriggerID); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("another tenant's read: err = %v, want sql.ErrNoRows", err)
	}
}

func TestFacadeRecordsWhyAnAdmittedItemWasRefusedOrExpired(t *testing.T) {
	t.Parallel()
	db, _ := processed(t)
	var itemID string
	if err := db.QueryRowContext(t.Context(), "SELECT scheduler_item_id FROM scheduler_items").Scan(&itemID); err != nil {
		t.Fatal(err)
	}
	err := db.WithTx(t.Context(), func(tx *sql.Tx) error {
		if err := cognition.RecordCostRejectionReason(t.Context(), tx, itemID, errors.New("budget exhausted")); err != nil {
			return err
		}
		return cognition.RecordSchedulerExpiryReason(t.Context(), tx, itemID, "unreadable expires_at")
	})
	if err != nil {
		t.Fatal(err)
	}
	listed, err := cognition.TriggerEvaluations(t.Context(), db.DB, "tenant", "sit-1", 1)
	if err != nil || len(listed) != 1 || len(listed[0].Reasons) != 3 {
		t.Fatalf("evaluations = %+v, %v; want admission, cost refusal and expiry reasons", listed, err)
	}
}
