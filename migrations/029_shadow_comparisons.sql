-- Phase 02: paired baseline/Tamoz shadow evidence. This table is report-only;
-- it has no foreign key into intents, commands, or outbox and is never an
-- authorization source.
CREATE TABLE IF NOT EXISTS shadow_comparisons (
    comparison_id              TEXT PRIMARY KEY,
    comparison_key             TEXT NOT NULL UNIQUE,
    tenant_id                  TEXT NOT NULL,
    episode_id                 TEXT NOT NULL,
    situation_id               TEXT NOT NULL,
    situation_version          INTEGER NOT NULL CHECK (situation_version >= 1),
    trigger_id                 TEXT NOT NULL,
    snapshot_sha256            BLOB NOT NULL CHECK (length(snapshot_sha256) = 32),
    spec_sha256                BLOB NOT NULL CHECK (length(spec_sha256) = 32),
    policy_sha256              BLOB NOT NULL CHECK (length(policy_sha256) = 32),
    baseline_executor_version  TEXT NOT NULL,
    tamoz_executor_version     TEXT NOT NULL,
    baseline_manifest_sha256   BLOB NOT NULL CHECK (length(baseline_manifest_sha256) = 32),
    tamoz_manifest_sha256      BLOB NOT NULL CHECK (length(tamoz_manifest_sha256) = 32),
    baseline_decision_json     BLOB NOT NULL,
    baseline_decision_sha256   BLOB NOT NULL CHECK (length(baseline_decision_sha256) = 32),
    tamoz_decision_json        BLOB NOT NULL,
    tamoz_decision_sha256      BLOB NOT NULL CHECK (length(tamoz_decision_sha256) = 32),
    comparison_json            BLOB NOT NULL,
    comparison_sha256          BLOB NOT NULL CHECK (length(comparison_sha256) = 32),
    created_at                 TEXT NOT NULL,
    FOREIGN KEY (episode_id) REFERENCES episodes(episode_id)
) STRICT;

CREATE INDEX shadow_comparisons_situation
    ON shadow_comparisons (tenant_id, situation_id, situation_version);
