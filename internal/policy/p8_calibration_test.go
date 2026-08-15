package policy

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/ids"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
)

// P8 (docs/new-design/PHASE_P8_ROLLOUT.md): calibration-gated automation. An
// automatic consequential intent (R2+) is refused — falling back to the
// human-approval path (watch-only) — until an exact calibration artifact
// exists for the domain (model revision = the compiled-spec digest + domain).
// R0/R1 automatic intents are unaffected.

func calibrationFixture(t *testing.T, risk, domain, modelRevision, artifactSHA string) (*storage.DB, string, *storage.CalibrationStore) {
	t.Helper()
	db, intentID := openPolicyFixture(t, risk, 1, 1, time.Date(2099, 1, 1, 0, 0, 0, 0, time.UTC))
	// The calibration gate needs the episode's model revision and the
	// situation's domain — the fixture defaults them empty, so pin them.
	if _, err := db.ExecContext(context.Background(),
		"UPDATE episodes SET executor_version = ? WHERE episode_id = 'epi-policy'", modelRevision); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(context.Background(),
		"UPDATE situations SET situation_type = ? WHERE situation_id = 'sit-policy'", domain); err != nil {
		t.Fatal(err)
	}
	store := &storage.CalibrationStore{DB: db}
	if artifactSHA != "" {
		if err := store.Activate(context.Background(), storage.CalibrationArtifact{
			Domain: domain, ModelRevision: modelRevision, ArtifactSHA256: artifactSHA,
		}, artifactSHA); err != nil {
			t.Fatal(err)
		}
	}
	return db, intentID, store
}

// An R2 automatic intent with NO calibration artifact is refused — it falls
// through to the human-approval path (watch-only), never to silent automation.
func TestP8R2AutomaticWithoutCalibrationIsWatchOnly(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 8, 14, 12, 0, 0, 0, time.UTC)
	db, intentID, store := calibrationFixture(t, "R2", "test", "sha256:1111", "")
	defer func() { _ = db.Close() }()

	gateway := NewGateway("policy-v1", ids.Deterministic()).WithCalibration(store)
	var result Result
	if err := db.WithTx(ctx, func(tx *sql.Tx) error {
		var err error
		result, err = gateway.EvaluateIntent(ctx, tx, intentID, now)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if result.Result != "approval_required" {
		t.Fatalf("R2 without calibration must be watch-only (approval_required), got %s/%s",
			result.Result, result.Reason)
	}
}

// An R2 automatic intent WITH the exact artifact is auto-approved via the
// calibrated-automation path.
func TestP8R2AutomaticWithExactCalibrationIsApproved(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 8, 14, 12, 0, 0, 0, time.UTC)
	db, intentID, store := calibrationFixture(t, "R2", "test", "sha256:1111", "sha256:1111")
	defer func() { _ = db.Close() }()

	gateway := NewGateway("policy-v1", ids.Deterministic()).WithCalibration(store)
	var result Result
	if err := db.WithTx(ctx, func(tx *sql.Tx) error {
		var err error
		result, err = gateway.EvaluateIntent(ctx, tx, intentID, now)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if result.Result != "approved" {
		t.Fatalf("R2 with exact calibration must be approved, got %s/%s", result.Result, result.Reason)
	}
	if result.Reason != "calibrated_automation" {
		t.Fatalf("approved reason = %q, want calibrated_automation", result.Reason)
	}
}

// A MISMATCHED artifact (model revision changed) is refused — missing or
// mismatched = watch-only.
func TestP8R2AutomaticWithMismatchedCalibrationIsWatchOnly(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 8, 14, 12, 0, 0, 0, time.UTC)
	// Activate an artifact for revision 1111, then deploy a NEW spec revision
	// (2222) for the same domain: the compiled-spec digest changed, so the
	// artifact no longer matches — watch-only until re-registered.
	db, intentID, store := calibrationFixture(t, "R2", "test", "sha256:1111", "sha256:1111")
	defer func() { _ = db.Close() }()
	if _, err := db.ExecContext(context.Background(),
		"UPDATE episodes SET executor_version = 'sha256:2222' WHERE episode_id = 'epi-policy'"); err != nil {
		t.Fatal(err)
	}

	gateway := NewGateway("policy-v1", ids.Deterministic()).WithCalibration(store)
	var result Result
	if err := db.WithTx(ctx, func(tx *sql.Tx) error {
		var err error
		result, err = gateway.EvaluateIntent(ctx, tx, intentID, now)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if result.Result != "approval_required" {
		t.Fatalf("mismatched calibration must be watch-only, got %s/%s", result.Result, result.Reason)
	}
}

// R1 automatic intents are NOT gated by calibration — watch-only active
// (R0/R1) is always allowed.
func TestP8R1AutomaticIgnoresCalibration(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 8, 14, 12, 0, 0, 0, time.UTC)
	db, intentID, store := calibrationFixture(t, "R1", "test", "sha256:1111", "")
	defer func() { _ = db.Close() }()

	gateway := NewGateway("policy-v1", ids.Deterministic()).WithCalibration(store)
	var result Result
	if err := db.WithTx(ctx, func(tx *sql.Tx) error {
		var err error
		result, err = gateway.EvaluateIntent(ctx, tx, intentID, now)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if result.Result != "approved" {
		t.Fatalf("R1 without calibration must be approved (watch-only active), got %s/%s",
			result.Result, result.Reason)
	}
}
