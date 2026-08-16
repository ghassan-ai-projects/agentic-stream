-- P8: calibration artifacts. Automatic consequential intents (R2+) are
-- refused until an exact artifact exists for the domain: the model revision,
-- the profile digest, the prompt, the diagnosis catalog, and the policy digest
-- all bound into one row. Missing or mismatched = watch-only.
CREATE TABLE IF NOT EXISTS calibration_artifacts (
    artifact_id             TEXT PRIMARY KEY,
    domain                  TEXT NOT NULL,
    model_revision          TEXT NOT NULL,
    profile_digest          TEXT NOT NULL,
    prompt_sha256           TEXT NOT NULL,
    diagnosis_catalog_sha256 TEXT NOT NULL,
    policy_digest           TEXT NOT NULL,
    artifact_sha256         TEXT NOT NULL,
    active                  INTEGER NOT NULL DEFAULT 1 CHECK (active IN (0, 1)),
    created_at              TEXT NOT NULL,
    UNIQUE (domain, artifact_sha256)
) STRICT;

CREATE INDEX calibration_artifacts_active ON calibration_artifacts (domain, active);
