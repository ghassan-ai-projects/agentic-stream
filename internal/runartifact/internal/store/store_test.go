package store_test

import (
	"bytes"
	"context"
	"slices"
	"strings"
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/actionport"
	"github.com/ghassan-ai-projects/agentic-stream/internal/runartifact/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/runartifact/internal/store"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage/storagetest"
	"github.com/ghassan-ai-projects/agentic-stream/migrations"
)

func snapshotOf(t *testing.T, db *storage.DB, read func(*store.Snapshot)) {
	t.Helper()
	if err := store.New(db).InSnapshot(t.Context(), func(snapshot *store.Snapshot) error {
		read(snapshot)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func seedRunDatabase(t *testing.T) *storage.DB {
	t.Helper()
	db := storagetest.OpenTempWithoutForeignKeys(t)
	for _, statement := range []string{
		`INSERT INTO event_log (tenant_id, partition_id, event_id, event_type, schema_version, source, partition_key, entity_type, entity_id, event_time, ingested_at, classification, quality_json, payload_json, payload_sha256, created_at)
			VALUES ('tenant', 0, 'evt-1', 't', '1', 's', 'k', 'e', 'x', '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z', 'internal', CAST('{}' AS BLOB), CAST('{}' AS BLOB), zeroblob(32), '2026-01-01T00:00:00Z'),
			       ('other', 0, 'evt-foreign', 't', '1', 's', 'k', 'e', 'x', '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z', 'internal', CAST('{}' AS BLOB), CAST('{}' AS BLOB), zeroblob(32), '2026-01-01T00:00:00Z'),
			       ('tenant', 0, 'evt-2', 't', '1', 's', 'k', 'e', 'x', '2026-01-01T00:00:01Z', '2026-01-01T00:00:01Z', 'internal', CAST('{}' AS BLOB), CAST('{}' AS BLOB), zeroblob(32), '2026-01-01T00:00:01Z')`,
		`INSERT INTO spec_deployments (deployment_id, tenant_id, spec_name, spec_version, spec_schema_version, spec_sha256, source_json, compiled_ir, status, created_at)
			VALUES ('dep-old', 'tenant', 'n-old', '1', '1', x'0000000000000000000000000000000000000000000000000000000000000001', CAST('{"v":"old"}' AS BLOB), x'00', 'active', '2026-01-01T00:00:00Z'),
			       ('dep-new', 'tenant', 'n-new', '2', '1', x'0000000000000000000000000000000000000000000000000000000000000002', CAST('{"v":"new"}' AS BLOB), x'00', 'active', '2026-01-02T00:00:00Z'),
			       ('dep-foreign', 'other', 'n', '3', '1', x'0000000000000000000000000000000000000000000000000000000000000003', CAST('{"v":"foreign"}' AS BLOB), x'00', 'active', '2026-01-03T00:00:00Z')`,
		`INSERT INTO intents (intent_id, decision_id, tenant_id, situation_id, situation_version, intent_type, risk_class, intent_json, intent_sha256, expires_at, policy_status, created_at, updated_at, rate_limit_per_hour, requires_approval)
			VALUES ('int-1', 'dec-1', 'tenant', 's', 1, 't', 'R1', x'00', zeroblob(32), '2099-01-01T00:00:00Z', 'approved', '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z', 0, 0),
			       ('int-2', 'dec-2', 'tenant', 's', 1, 't', 'R1', x'00', zeroblob(32), '2099-01-01T00:00:00Z', 'approved', '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z', 0, 0),
			       ('int-3', 'dec-3', 'other', 's', 1, 't', 'R1', x'00', zeroblob(32), '2099-01-01T00:00:00Z', 'approved', '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z', 0, 0)`,
		`INSERT INTO policy_evaluations (evaluation_id, intent_id, decision_id, policy_version, policy_digest, intent_sha256, decision_sha256, result, reason, situation_version, evaluated_at)
			VALUES ('ev-old', 'int-1', 'dec-1', 'policy-old', 'sha256:old', zeroblob(32), zeroblob(32), 'approved', 'r', 1, '2026-01-01T00:00:00Z'),
			       ('ev-new', 'int-1', 'dec-1', 'policy-new', 'sha256:new', zeroblob(32), zeroblob(32), 'approved', 'r', 1, '2026-01-02T00:00:00Z')`,
		`INSERT INTO commands (command_id, intent_id, tenant_id, effector_route, normalized_target, idempotency_key, command_json, command_sha256, status, created_at, updated_at)
			VALUES ('cmd-done', 'int-1', 'tenant', 'r', 't1', x'0000000000000000000000000000000000000000000000000000000000000001', x'00', zeroblob(32), '` + actionport.CommandSucceeded + `', '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z'),
			       ('cmd-unresolved', 'int-2', 'tenant', 'r', 't2', x'0000000000000000000000000000000000000000000000000000000000000002', x'00', zeroblob(32), '` + actionport.CommandReconciling + `', '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z'),
			       ('cmd-foreign', 'int-3', 'other', 'r', 't3', x'0000000000000000000000000000000000000000000000000000000000000003', x'00', zeroblob(32), '` + actionport.CommandReconciling + `', '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z')`,
		`INSERT INTO verifications (verification_id, intent_id, command_id, status, updated_at)
			VALUES ('ver-1', 'int-1', 'cmd-done', '` + actionport.VerificationAwaiting + `', '2026-01-01T00:00:00Z')`,
		`INSERT INTO device_reconciliation (device_id, boot_id, status, state_json, state_sha256, owner_epoch, first_seen_at, updated_at)
			VALUES ('dev-b', 'boot', 'clear', CAST('{"device":"b"}' AS BLOB), zeroblob(32), 'epoch', '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z'),
			       ('dev-a', 'boot', 'clear', CAST('{"device":"a"}' AS BLOB), zeroblob(32), 'epoch', '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z')`,
	} {
		if _, err := db.ExecContext(t.Context(), statement); err != nil {
			t.Fatalf("seed run database: %v\n%s", err, statement)
		}
	}
	return db
}

func columnValues(t *testing.T, table domain.LedgerTable, column string) []string {
	t.Helper()
	index := slices.Index(table.Columns, column)
	if index < 0 {
		t.Fatalf("ledger has no column %q: %v", column, table.Columns)
	}
	values := make([]string, 0, len(table.Rows))
	for _, row := range table.Rows {
		values = append(values, row[index].(string))
	}
	return values
}

func TestEveryLedgerFileCanBeReadFromAnEmptyRunDatabase(t *testing.T) {
	t.Parallel()
	snapshotOf(t, storagetest.OpenTemp(t), func(snapshot *store.Snapshot) {
		for _, name := range domain.LedgerFiles() {
			table, err := snapshot.Ledger(t.Context(), name, "tenant")
			if err != nil || len(table.Columns) == 0 || len(table.Rows) != 0 {
				t.Errorf("ledger %s = %+v, %v; want its columns and no rows", name, table, err)
			}
		}
	})
}

func TestLedgerReturnsOnlyTheTenantsRowsInPositionOrder(t *testing.T) {
	t.Parallel()
	snapshotOf(t, seedRunDatabase(t), func(snapshot *store.Snapshot) {
		table, err := snapshot.Ledger(t.Context(), domain.FileObservations, "tenant")
		if err != nil {
			t.Fatal(err)
		}
		if got := columnValues(t, table, "event_id"); !slices.Equal(got, []string{"evt-1", "evt-2"}) {
			t.Fatalf("observation ledger = %v, want the tenant's events in position order", got)
		}
		commands, err := snapshot.Ledger(t.Context(), domain.FileCommands, "tenant")
		if err != nil {
			t.Fatal(err)
		}
		if got := columnValues(t, commands, "command_id"); !slices.Equal(got, []string{"cmd-done", "cmd-unresolved"}) {
			t.Fatalf("command ledger = %v, want the tenant's commands by id", got)
		}
	})
}

func TestLedgerRefusesAFileThatIsNotPartOfTheArtifact(t *testing.T) {
	t.Parallel()
	snapshotOf(t, storagetest.OpenTemp(t), func(snapshot *store.Snapshot) {
		if _, err := snapshot.Ledger(t.Context(), "unknown.jsonl", "tenant"); err == nil || !strings.Contains(err.Error(), `unknown ledger file "unknown.jsonl"`) {
			t.Fatalf("an unknown ledger file = %v, want unknown ledger file", err)
		}
	})
}

var wantNewDigest = append(make([]byte, 31), 2)

func TestSnapshotReadsTheNewestDeploymentAndPolicyOfTheTenant(t *testing.T) {
	t.Parallel()
	snapshotOf(t, seedRunDatabase(t), func(snapshot *store.Snapshot) {
		ctx := t.Context()
		if digest, err := snapshot.LatestSpecDigest(ctx, "tenant"); err != nil || !bytes.Equal(digest, wantNewDigest) {
			t.Fatalf("latest spec digest = %x, %v; want the newest deployment's", digest, err)
		}
		if source, err := snapshot.LatestSpecSource(ctx, "tenant"); err != nil || string(source) != `{"v":"new"}` {
			t.Fatalf("latest spec source = %s, %v; want the newest deployment's", source, err)
		}
		if digest, err := snapshot.LatestSpecDigest(ctx, "nobody"); err != nil || digest != nil {
			t.Fatalf("a tenant without deployments = %x, %v; want none", digest, err)
		}
		if evaluation, err := snapshot.LatestPolicyEvaluation(ctx, "tenant"); err != nil || evaluation != (store.PolicyEvaluation{Version: "policy-new", Digest: "sha256:new"}) {
			t.Fatalf("latest policy evaluation = %+v, %v; want the newest", evaluation, err)
		}
		if evaluation, err := snapshot.LatestPolicyEvaluation(ctx, "nobody"); err != nil || evaluation != (store.PolicyEvaluation{}) {
			t.Fatalf("a tenant without evaluations = %+v, %v; want the zero value", evaluation, err)
		}
	})
}

func TestSnapshotReadsDeviceStateByIdOrTheFirstDevice(t *testing.T) {
	t.Parallel()
	snapshotOf(t, seedRunDatabase(t), func(snapshot *store.Snapshot) {
		ctx := t.Context()
		if count, err := snapshot.DeviceStateCount(ctx); err != nil || count != 2 {
			t.Fatalf("device state count = %d, %v; want 2", count, err)
		}
		if state, err := snapshot.DeviceState(ctx, ""); err != nil || string(state) != `{"device":"a"}` {
			t.Fatalf("first device state = %s, %v; want device a", state, err)
		}
		if state, err := snapshot.DeviceState(ctx, "dev-b"); err != nil || string(state) != `{"device":"b"}` {
			t.Fatalf("named device state = %s, %v; want device b", state, err)
		}
		if state, err := snapshot.DeviceState(ctx, "dev-missing"); err != nil || state != nil {
			t.Fatalf("an unreported device = %s, %v; want none", state, err)
		}
	})
}

func TestSafetyEvidenceCountsTheTenantsActionOutcomes(t *testing.T) {
	t.Parallel()
	snapshotOf(t, seedRunDatabase(t), func(snapshot *store.Snapshot) {
		evidence, err := snapshot.SafetyEvidence(t.Context(), "tenant")
		if err != nil {
			t.Fatal(err)
		}
		want := map[string]uint64{"commands": 2, "unknown_outcomes": 1, "awaiting_verification": 1, "unresolved_action_outcomes": 2}
		for name, count := range want {
			if evidence.ActionDiagnostics[name] != count {
				t.Errorf("%s = %d, want %d (all: %v)", name, evidence.ActionDiagnostics[name], count, evidence.ActionDiagnostics)
			}
		}
	})
}

func TestSnapshotReportsTheAppliedMigrationVersion(t *testing.T) {
	t.Parallel()
	all, err := migrations.All()
	if err != nil {
		t.Fatal(err)
	}
	snapshotOf(t, storagetest.OpenTemp(t), func(snapshot *store.Snapshot) {
		if version, err := snapshot.MigrationVersion(t.Context()); err != nil || version != all[len(all)-1].Version {
			t.Fatalf("migration version = %d, %v; want %d", version, err, all[len(all)-1].Version)
		}
	})
}

func TestStoreWithoutADatabaseIsZeroAndASnapshotNeedsOne(t *testing.T) {
	t.Parallel()
	if !store.New(nil).IsZero() || store.New(storagetest.OpenTemp(t)).IsZero() {
		t.Fatal("IsZero does not tell a store without a database from one with a database")
	}
	failure := store.New(storagetest.OpenTemp(t)).InSnapshot(canceledContext(t), func(*store.Snapshot) error { return nil })
	if failure == nil || !strings.Contains(failure.Error(), "begin run artifact snapshot") {
		t.Fatalf("a snapshot under a canceled context = %v, want begin run artifact snapshot", failure)
	}
}

func canceledContext(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	return ctx
}
