package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// ErrCalibrationMissing means an automatic consequential intent was attempted
// without an exact calibration artifact for the domain — watch-only.
var ErrCalibrationMissing = errors.New("calibration artifact missing or mismatched")

// CalibrationArtifact is the exact bound artifact that unlocks automatic
// consequential intents (R2+) for one domain.
type CalibrationArtifact struct {
	Domain               string
	ModelRevision        string
	ProfileDigest        string
	PromptSHA256         string
	DiagnosisCatalogSHA  string
	PolicyDigest         string
	ArtifactSHA256       string
}

// CalibrationStore persists and asserts the per-domain calibration artifacts.
type CalibrationStore struct {
	DB *DB
}

// AssertCalibration fails closed when no ACTIVE artifact matches the domain's
// model revision. The model revision is the compiled-spec digest, which binds
// prompt + diagnosis catalog + policy — a spec change therefore invalidates
// the artifact (watch-only). The artifact row's profile digest and artifact
// SHA are the Ruby-bound document identity, verified at ACTIVATION (the
// operator registers the generated artifact against the running spec); the
// gate itself checks what the runtime can verify from its own rows.
func (s *CalibrationStore) AssertCalibration(ctx context.Context, tx *sql.Tx, artifact CalibrationArtifact) error {
	var ignored int
	err := tx.QueryRowContext(ctx, `
		SELECT active FROM calibration_artifacts
		WHERE domain = ? AND model_revision = ?
		      AND active = 1`,
		artifact.Domain, artifact.ModelRevision,
	).Scan(&ignored)
	if errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("%w: %s", ErrCalibrationMissing, artifact.Domain)
	}
	if err != nil {
		return fmt.Errorf("read calibration artifact: %w", err)
	}
	return nil
}

// Activate registers an artifact as the active one for the domain (one active
// per domain — a new artifact deactivates the previous).
func (s *CalibrationStore) Activate(ctx context.Context, artifact CalibrationArtifact, artifactSHA256 string) error {
	if s == nil || s.DB == nil || artifact.Domain == "" {
		return fmt.Errorf("calibration store is not configured")
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	return s.DB.WithTx(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx,
			`UPDATE calibration_artifacts SET active = 0 WHERE domain = ?`, artifact.Domain); err != nil {
			return fmt.Errorf("deactivate prior calibration: %w", err)
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO calibration_artifacts (
				artifact_id, domain, model_revision, profile_digest, prompt_sha256,
				diagnosis_catalog_sha256, policy_digest, artifact_sha256, active, created_at
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?, 1, ?)
			ON CONFLICT(domain, artifact_sha256) DO UPDATE SET active = 1`,
			artifact.Domain+"-"+shortDigest(artifactSHA256), artifact.Domain,
			artifact.ModelRevision, artifact.ProfileDigest, artifact.PromptSHA256,
			artifact.DiagnosisCatalogSHA, artifact.PolicyDigest, artifactSHA256, now); err != nil {
			return fmt.Errorf("activate calibration artifact: %w", err)
		}
		return nil
	})
}

func shortDigest(digest string) string {
	if len(digest) > 12 {
		return digest[:12]
	}
    return digest
}
