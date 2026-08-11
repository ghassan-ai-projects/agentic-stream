PRAGMA foreign_keys = ON;

CREATE TABLE IF NOT EXISTS situation_models (
    id                    TEXT NOT NULL,
    version               TEXT NOT NULL,
    entity_type           TEXT NOT NULL,
    status                TEXT NOT NULL CHECK (
        status IN ('installed', 'active', 'retired')
    ),
    source_json           TEXT NOT NULL CHECK (json_valid(source_json)),
    normalized_json       TEXT NOT NULL CHECK (json_valid(normalized_json)),
    digest                TEXT NOT NULL UNIQUE,
    compiler_version      TEXT NOT NULL,
    installed_at_ns       INTEGER NOT NULL,
    activated_at_ns       INTEGER,
    PRIMARY KEY (id, version),
    UNIQUE (id, version, digest)
);

CREATE UNIQUE INDEX IF NOT EXISTS situation_models_one_active_entity_idx
    ON situation_models(entity_type)
    WHERE status = 'active';

CREATE TABLE IF NOT EXISTS events (
    id                TEXT PRIMARY KEY,
    entity_type       TEXT NOT NULL,
    entity_id         TEXT NOT NULL,
    type              TEXT NOT NULL,
    event_time_ns     INTEGER NOT NULL,
    arrival_time_ns   INTEGER NOT NULL,
    value_type        TEXT NOT NULL CHECK (
        value_type IN ('number', 'string', 'none')
    ),
    numeric_value     REAL,
    text_value        TEXT,
    unit              TEXT,
    payload_hash      TEXT NOT NULL,
    model_digest      TEXT NOT NULL,
    late_ignored      INTEGER NOT NULL DEFAULT 0 CHECK (late_ignored IN (0, 1)),
    ignore_reason     TEXT,
    created_at_ns     INTEGER NOT NULL,
    CHECK (
        (value_type = 'number' AND numeric_value IS NOT NULL AND text_value IS NULL)
        OR
        (value_type = 'string' AND numeric_value IS NULL AND text_value IS NOT NULL)
        OR
        (value_type = 'none' AND numeric_value IS NULL AND text_value IS NULL)
    ),
    FOREIGN KEY (model_digest) REFERENCES situation_models(digest)
);

CREATE INDEX IF NOT EXISTS events_entity_time_idx
    ON events(entity_type, entity_id, event_time_ns);

CREATE INDEX IF NOT EXISTS events_entity_arrival_idx
    ON events(entity_type, entity_id, arrival_time_ns);

CREATE TABLE IF NOT EXISTS entity_state (
    entity_type          TEXT NOT NULL,
    entity_id            TEXT NOT NULL,
    model_digest         TEXT NOT NULL,
    event_horizon_ns     INTEGER NOT NULL,
    current_state        TEXT NOT NULL,
    situation_version    INTEGER NOT NULL DEFAULT 0,
    facts_json           TEXT NOT NULL CHECK (json_valid(facts_json)),
    fact_bands_json      TEXT NOT NULL CHECK (json_valid(fact_bands_json)),
    updated_at_ns        INTEGER NOT NULL,
    PRIMARY KEY (entity_type, entity_id),
    FOREIGN KEY (model_digest) REFERENCES situation_models(digest)
);

CREATE TABLE IF NOT EXISTS situations (
    entity_type          TEXT NOT NULL,
    entity_id            TEXT NOT NULL,
    version              INTEGER NOT NULL,
    state                TEXT NOT NULL,
    previous_state       TEXT,
    event_horizon_ns     INTEGER NOT NULL,
    model_id             TEXT NOT NULL,
    model_version        TEXT NOT NULL,
    model_digest         TEXT NOT NULL,
    runtime_version      TEXT NOT NULL,
    facts_json           TEXT NOT NULL CHECK (json_valid(facts_json)),
    evidence_ids_json    TEXT NOT NULL CHECK (json_valid(evidence_ids_json)),
    evaluation_json      TEXT NOT NULL CHECK (json_valid(evaluation_json)),
    trigger_ids_json     TEXT NOT NULL CHECK (json_valid(trigger_ids_json)),
    canonical_hash       TEXT NOT NULL,
    created_at_ns        INTEGER NOT NULL,
    PRIMARY KEY (entity_type, entity_id, version),
    FOREIGN KEY (entity_type, entity_id)
        REFERENCES entity_state(entity_type, entity_id),
    FOREIGN KEY (model_id, model_version, model_digest)
        REFERENCES situation_models(id, version, digest)
);

CREATE INDEX IF NOT EXISTS situations_state_idx
    ON situations(state, created_at_ns);

CREATE TABLE IF NOT EXISTS episodes (
    id                    TEXT PRIMARY KEY,
    entity_type           TEXT NOT NULL,
    entity_id             TEXT NOT NULL,
    situation_version     INTEGER NOT NULL,
    trigger_id            TEXT NOT NULL,
    episode_type          TEXT NOT NULL,
    status                TEXT NOT NULL CHECK (
        status IN ('pending', 'running', 'completed', 'failed')
    ),
    snapshot_json         TEXT NOT NULL CHECK (json_valid(snapshot_json)),
    snapshot_hash         TEXT NOT NULL,
    attempt_count         INTEGER NOT NULL DEFAULT 0,
    started_at_ns         INTEGER,
    finished_at_ns        INTEGER,
    error_code            TEXT,
    error_detail_redacted TEXT,
    created_at_ns         INTEGER NOT NULL,
    UNIQUE (entity_type, entity_id, situation_version, trigger_id),
    FOREIGN KEY (entity_type, entity_id, situation_version)
        REFERENCES situations(entity_type, entity_id, version)
);

CREATE INDEX IF NOT EXISTS episodes_status_created_idx
    ON episodes(status, created_at_ns);

CREATE TABLE IF NOT EXISTS decisions (
    id                    TEXT PRIMARY KEY,
    episode_id            TEXT NOT NULL UNIQUE,
    output_json           TEXT NOT NULL CHECK (json_valid(output_json)),
    output_schema_digest  TEXT NOT NULL,
    evidence_ids_json     TEXT NOT NULL CHECK (json_valid(evidence_ids_json)),
    reasoning_provider    TEXT NOT NULL,
    reasoning_model       TEXT NOT NULL,
    situation_model_digest TEXT NOT NULL,
    created_at_ns         INTEGER NOT NULL,
    FOREIGN KEY (situation_model_digest) REFERENCES situation_models(digest),
    FOREIGN KEY (episode_id) REFERENCES episodes(id)
);
