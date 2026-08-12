CREATE TABLE event_schemas (
    schema_id       TEXT PRIMARY KEY,
    event_type      TEXT NOT NULL,
    schema_version  TEXT NOT NULL,
    schema_json     BLOB NOT NULL,
    schema_sha256   BLOB NOT NULL CHECK (length(schema_sha256) = 32),
    status          TEXT NOT NULL CHECK (status IN ('active', 'retired')),
    created_at      TEXT NOT NULL,
    UNIQUE (event_type, schema_version)
) STRICT;

CREATE INDEX event_schemas_lookup
    ON event_schemas(event_type, schema_version, status);

CREATE TABLE event_quarantine (
    quarantine_id   TEXT PRIMARY KEY,
    tenant_id       TEXT NOT NULL,
    event_id        TEXT NOT NULL,
    event_type      TEXT NOT NULL,
    schema_version  TEXT NOT NULL,
    source          TEXT NOT NULL,
    reason_code     TEXT NOT NULL,
    payload_json    BLOB NOT NULL,
    payload_sha256  BLOB NOT NULL CHECK (length(payload_sha256) = 32),
    attempt_count   INTEGER NOT NULL DEFAULT 1 CHECK (attempt_count >= 1),
    status          TEXT NOT NULL CHECK (status IN ('quarantined', 'released', 'rejected')),
    first_seen_at   TEXT NOT NULL,
    last_seen_at    TEXT NOT NULL,
    released_at     TEXT,
    UNIQUE (tenant_id, event_id)
) STRICT;

CREATE INDEX event_quarantine_status
    ON event_quarantine(tenant_id, status, last_seen_at);

CREATE TABLE event_gaps (
    gap_id          TEXT PRIMARY KEY,
    tenant_id       TEXT NOT NULL,
    partition_id    INTEGER NOT NULL CHECK (partition_id >= 0),
    from_position   INTEGER NOT NULL CHECK (from_position >= 0),
    to_position     INTEGER NOT NULL CHECK (to_position >= from_position),
    reason_code     TEXT NOT NULL,
    created_at      TEXT NOT NULL,
    resolved_at     TEXT
) STRICT;
