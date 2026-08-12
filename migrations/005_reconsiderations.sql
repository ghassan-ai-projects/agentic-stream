-- Durable deduplication for correction-triggered reconsideration episodes.

ALTER TABLE scheduler_items ADD COLUMN kind TEXT NOT NULL DEFAULT 'standard'
    CHECK (kind IN ('standard', 'reconsider'));

CREATE TABLE reconsiderations (
    reconsideration_id       TEXT PRIMARY KEY,
    tenant_id                TEXT NOT NULL,
    situation_id             TEXT NOT NULL,
    superseded_version       INTEGER NOT NULL CHECK (superseded_version >= 1),
    correction_version       INTEGER NOT NULL CHECK (correction_version > superseded_version),
    correction_snapshot_sha256 BLOB NOT NULL CHECK (length(correction_snapshot_sha256) = 32),
    invalidated_command_id   TEXT NOT NULL REFERENCES commands(command_id),
    invalidated_outcome_id   TEXT NOT NULL REFERENCES outcomes(outcome_id),
    invalidated_outcome_sha256 BLOB NOT NULL CHECK (length(invalidated_outcome_sha256) = 32),
    trigger_id               TEXT UNIQUE REFERENCES trigger_evaluations(trigger_id),
    scheduler_item_id        TEXT UNIQUE REFERENCES scheduler_items(scheduler_item_id),
    created_at               TEXT NOT NULL,
    UNIQUE (situation_id, superseded_version, invalidated_command_id)
) STRICT;

CREATE INDEX reconsiderations_situation
    ON reconsiderations(situation_id, correction_version, created_at);
