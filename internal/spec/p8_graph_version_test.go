package spec_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/spec"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
)

// P8 (docs/new-design/PHASE_P8_ROLLOUT.md): graph versioning. A graph
// definition change = a new spec version (the compiled digest is the version
// identity) + fresh state. Two versions of the same name must share NO
// situation/episode rows, and the new version's admission never touches the
// old version's episodes.

// Re-saving the SAME digest is idempotent: the active deployment stays active
// (a boot-time SaveDeployment must not retire its own row).
func TestP8SameDigestRedeployKeepsTheDeploymentActive(t *testing.T) {
	ctx := context.Background()
	db, err := storage.Open(ctx, filepath.Join(t.TempDir(), "redeploy.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()

	specDoc := p8VersionedSpec(t, "graph-test", "1.0", "sha256:0000000000000000000000000000000000000000000000000000000000000003")
	if err := spec.SaveDeployment(ctx, db, "tenant", specDoc); err != nil {
		t.Fatal(err)
	}
	// A second boot with the same digest: the row must stay active.
	if err := spec.SaveDeployment(ctx, db, "tenant", specDoc); err != nil {
		t.Fatal(err)
	}
	var status string
	if err := db.QueryRowContext(ctx, `
		SELECT status FROM spec_deployments WHERE deployment_id = ?`,
		specDoc.Digest).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != "active" {
		t.Fatalf("same-digest redeploy must keep the deployment active, got %q", status)
	}
}

func TestP8GraphVersionChangeIsAFreshNamespace(t *testing.T) {
	ctx := context.Background()
	db, err := storage.Open(ctx, filepath.Join(t.TempDir(), "graph-version.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()

	base := p8VersionedSpec(t, "graph-test", "1.0", "sha256:0000000000000000000000000000000000000000000000000000000000000001")
	next := p8VersionedSpec(t, "graph-test", "2.0", "sha256:0000000000000000000000000000000000000000000000000000000000000002")
	if err := spec.SaveDeployment(ctx, db, "tenant", base); err != nil {
		t.Fatal(err)
	}
	if err := spec.SaveDeployment(ctx, db, "tenant", next); err != nil {
		t.Fatal(err)
	}

	// Two distinct deployments for the same spec name — the digest is the
	// version identity.
	var deployments int
	if err := db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM spec_deployments WHERE tenant_id = 'tenant' AND spec_name = 'graph-test'`).Scan(&deployments); err != nil {
		t.Fatal(err)
	}
	if deployments != 2 {
		t.Fatalf("expected two deployments for the two versions, got %d", deployments)
	}

	// State rows are keyed by deployment_id: a situation admitted under v1
	// belongs to v1's deployment, never v2's.
	digestV1 := make([]byte, 32)
	digestV1[31] = 1
	if _, err := db.ExecContext(ctx, `
		INSERT INTO lineage_sets (lineage_id, sha256, reference_count, references_json, created_at)
		VALUES ('lin-v1', ?, 1, X'7B7D', '2026-08-12T00:00:00Z')`, digestV1); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `
		INSERT INTO situations (
			situation_id, tenant_id, deployment_id, situation_type, entity_type, entity_id,
			partition_id, occurrence_id, current_version, phase, status,
			first_event_time, latest_event_time, updated_at, created_at
		) VALUES ('sit-v1', 'tenant', ?, 'test', 'thing', 'ent-1', 0, 'occ-v1', 1, 'watch', 'open', ?, ?, ?, ?)`,
		base.Digest, "2026-08-12T00:00:00Z", "2026-08-12T00:00:00Z", "2026-08-12T00:00:00Z", "2026-08-12T00:00:00Z"); err != nil {
		t.Fatal(err)
	}

	var v1Deployment, v2Deployment string
	if err := db.QueryRowContext(ctx, "SELECT deployment_id FROM situations WHERE situation_id = 'sit-v1'").Scan(&v1Deployment); err != nil {
		t.Fatal(err)
	}
	if v1Deployment != base.Digest {
		t.Fatalf("v1 situation bound to deployment %q, want the v1 digest", v1Deployment)
	}
	// The v2 deployment has NO situation rows — a version change is a fresh
	// namespace, never a shim over v1's rows.
	if err := db.QueryRowContext(ctx,
		"SELECT deployment_id FROM situations WHERE deployment_id = ?", next.Digest).Scan(&v2Deployment); err == nil {
		t.Fatalf("v2 must not share any situation rows with v1 (found %q)", v2Deployment)
	}
}

func p8VersionedSpec(t *testing.T, name, version, digest string) *spec.CompiledSpec {
	t.Helper()
	return &spec.CompiledSpec{
		SchemaVersion: "agentic-stream/v1",
		Metadata:      spec.Metadata{Name: name, Version: version},
		Digest:        digest,
		Inputs: []spec.Input{{Name: "level", EventType: "test.observed", SchemaVersion: "1.0",
			PartitionKey: "entity.id", EntityType: "thing", Classification: "internal"}},
		Time:    spec.TimePolicy{MaxOutOfOrderness: "1m"},
		Situation: spec.Situation{Type: "test", InitialPhase: "candidate",
			Phases: []spec.Phase{{Name: "candidate", Severity: 10}},
			Occurrence: spec.Occurrence{OpenWhen: "features.level > 10"}},
		Cognition: spec.Cognition{
			Triggers: []spec.Trigger{{Name: "high", When: "features.level > 10", Score: "situation.severity", Threshold: 5, Lane: "fast"}},
			Executor: spec.Executor{Name: "native", ModelPolicy: "test", PromptVersion: "v1",
				DecisionSchema: "schemas/decision.json", Budget: spec.Budget{WallTime: "5s"}},
		},
	}
}
