package app_test

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/policy"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
)

// Consequential R2 intents need either an exact calibration artifact or human
// approval. Calibration binds the Situation type and episode executor version.
// Low-risk R0/R1 intents do not require calibration; R3/R4 remain denied.

func calibrationFixture(t *testing.T, risk, domain, modelRevision, artifactSHA string) (*storage.DB, string) {
	t.Helper()
	db, intentID := openPolicyFixture(t, risk, 1, 1, time.Date(2099, 1, 1, 0, 0, 0, 0, time.UTC))
	// The calibration gate needs the episode's model revision and the
	// situation's domain, so pin both values for this case.
	if _, err := db.ExecContext(context.Background(),
		"UPDATE episodes SET executor_version = ? WHERE episode_id = 'epi-policy'", modelRevision); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(context.Background(),
		"UPDATE situations SET situation_type = ? WHERE situation_id = 'sit-policy'", domain); err != nil {
		t.Fatal(err)
	}
	if artifactSHA != "" {
		activateCalibration(t, db, domain, modelRevision, artifactSHA)
	}
	return db, intentID
}

// activateCalibration provisions an active artifact the way an operator would:
// the runtime only reads calibration_artifacts.
func activateCalibration(t *testing.T, db *storage.DB, domain, modelRevision, artifactSHA string) {
	t.Helper()
	if _, err := db.ExecContext(context.Background(), `
		INSERT INTO calibration_artifacts (
			artifact_id, domain, model_revision, profile_digest, prompt_sha256,
			diagnosis_catalog_sha256, policy_digest, artifact_sha256, active, created_at
		) VALUES (?, ?, ?, '', '', '', '', ?, 1, 'now')`,
		domain+"-"+artifactSHA, domain, modelRevision, artifactSHA); err != nil {
		t.Fatal(err)
	}
}

// Missing calibration routes R2 intents to human approval.
func TestConsequentialIntentWithoutCalibrationRequiresApproval(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 8, 14, 12, 0, 0, 0, time.UTC)
	db, intentID := calibrationFixture(t, "R2", "test", "sha256:1111", "")
	defer func() { _ = db.Close() }()

	gateway := newTestService(t)
	var result policy.Result
	if err := db.WithTx(ctx, func(tx *sql.Tx) error {
		var err error
		result, err = gateway.EvaluateIntent(ctx, tx, policy.EvaluationRequest{IntentID: intentID, Now: now})
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if result.Result != "approval_required" || result.CommandID != "" {
		t.Fatalf("R2 without calibration must require approval, got %s/%s",
			result.Result, result.Reason)
	}
}

// Matching calibration permits automatic R2 approval.
func TestConsequentialIntentWithExactCalibrationIsApproved(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 8, 14, 12, 0, 0, 0, time.UTC)
	db, intentID := calibrationFixture(t, "R2", "test", "sha256:1111", "sha256:1111")
	defer func() { _ = db.Close() }()

	gateway := newTestService(t)
	var result policy.Result
	if err := db.WithTx(ctx, func(tx *sql.Tx) error {
		var err error
		result, err = gateway.EvaluateIntent(ctx, tx, policy.EvaluationRequest{IntentID: intentID, Now: now})
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if result.Result != "approved" || result.CommandID == "" {
		t.Fatalf("R2 with exact calibration must be approved, got %s/%s", result.Result, result.Reason)
	}
	if result.Reason != "calibrated_automation" {
		t.Fatalf("approved reason = %q, want calibrated_automation", result.Reason)
	}
}

// A changed executor version requires fresh matching calibration.
func TestConsequentialIntentWithMismatchedCalibrationRequiresApproval(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 8, 14, 12, 0, 0, 0, time.UTC)
	// Activate revision 1111, then evaluate an episode from executor revision
	// 2222 for the same domain; the existing artifact no longer authorizes it.
	db, intentID := calibrationFixture(t, "R2", "test", "sha256:1111", "sha256:1111")
	defer func() { _ = db.Close() }()
	if _, err := db.ExecContext(context.Background(),
		"UPDATE episodes SET executor_version = 'sha256:2222' WHERE episode_id = 'epi-policy'"); err != nil {
		t.Fatal(err)
	}

	gateway := newTestService(t)
	var result policy.Result
	if err := db.WithTx(ctx, func(tx *sql.Tx) error {
		var err error
		result, err = gateway.EvaluateIntent(ctx, tx, policy.EvaluationRequest{IntentID: intentID, Now: now})
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if result.Result != "approval_required" || result.CommandID != "" {
		t.Fatalf("mismatched calibration must require approval, got %s/%s", result.Result, result.Reason)
	}
}

// R1 approval does not depend on the consequential calibration gate.
func TestLowRiskIntentDoesNotRequireCalibration(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 8, 14, 12, 0, 0, 0, time.UTC)
	db, intentID := calibrationFixture(t, "R1", "test", "sha256:1111", "")
	defer func() { _ = db.Close() }()

	gateway := newTestService(t)
	var result policy.Result
	if err := db.WithTx(ctx, func(tx *sql.Tx) error {
		var err error
		result, err = gateway.EvaluateIntent(ctx, tx, policy.EvaluationRequest{IntentID: intentID, Now: now})
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if result.Result != "approved" || result.CommandID == "" {
		t.Fatalf("R1 without calibration must be approved, got %s/%s",
			result.Result, result.Reason)
	}
}
