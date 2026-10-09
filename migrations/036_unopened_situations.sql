-- Runtime state of a Situation whose occurrence has not opened yet (no version
-- published), so a restart or rollback rebuilds the facts and evidence a
-- replay would hold. The row is removed when the Situation first publishes.
CREATE TABLE unopened_situations (
    situation_id        TEXT PRIMARY KEY,
    tenant_id           TEXT NOT NULL,
    deployment_id       TEXT NOT NULL REFERENCES spec_deployments(deployment_id),
    situation_type      TEXT NOT NULL,
    entity_type         TEXT NOT NULL,
    entity_id           TEXT NOT NULL,
    partition_id        INTEGER NOT NULL CHECK (partition_id >= 0),
    occurrence_id       TEXT NOT NULL,
    phase               TEXT NOT NULL,
    severity            INTEGER NOT NULL,
    confidence          REAL NOT NULL,
    completeness        TEXT NOT NULL,
    first_event_time    TEXT NOT NULL,
    latest_event_time   TEXT NOT NULL,
    traceparent         TEXT,
    tracestate          TEXT,
    state_json          BLOB NOT NULL,
    state_sha256        BLOB NOT NULL CHECK (length(state_sha256) = 32),
    updated_at          TEXT NOT NULL
) STRICT;
