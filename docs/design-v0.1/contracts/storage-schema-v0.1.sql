-- Agentic Stream V0.1 storage contract.
-- Seven tables. Changes from V0 are marked [F-nn] against CRITIQUE_OF_V0.md.
--
-- PRAGMA settings applied at open (not stored here):
--   journal_mode = WAL, foreign_keys = ON, busy_timeout = 5000, synchronous = NORMAL

PRAGMA foreign_keys = ON;

-- ---------------------------------------------------------------------------
-- situation_models
-- Immutable compiled domain models. The digest covers the complete normalized
-- document, including id and version. It is globally unique because dependent
-- records use it as the content-addressed foreign key.
-- ---------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS situation_models (
    id                    TEXT NOT NULL,
    version               TEXT NOT NULL,
    entity_type           TEXT NOT NULL,
    status                TEXT NOT NULL CHECK (
        status IN ('installed', 'active', 'retired')
    ),
    source_json           TEXT NOT NULL CHECK (json_valid(source_json)),
    normalized_json       TEXT NOT NULL CHECK (json_valid(normalized_json)),
    -- Compiler outputs kept for audit and for explain/compare without recompiling.
    bands_json            TEXT NOT NULL CHECK (json_valid(bands_json)),
    digest                TEXT NOT NULL,
    compiler_version      TEXT NOT NULL,
    -- Verdict against whatever was active for this entity_type at install time. [F-08]
    compatibility         TEXT NOT NULL CHECK (
        compatibility IN ('first', 'compatible', 'reset_required')
    ),
    compatibility_detail  TEXT,
    installed_at_ns       INTEGER NOT NULL,
    activated_at_ns       INTEGER,
    retired_at_ns         INTEGER,
    PRIMARY KEY (id, version),
    CHECK (length(digest) = 71 AND digest GLOB 'sha256:*'),
    CHECK (activated_at_ns IS NULL OR activated_at_ns >= installed_at_ns),
    CHECK (retired_at_ns IS NULL OR retired_at_ns >= installed_at_ns),
    CHECK (
        (status = 'installed' AND activated_at_ns IS NULL AND retired_at_ns IS NULL)
        OR (status = 'active' AND activated_at_ns IS NOT NULL AND retired_at_ns IS NULL)
        OR (status = 'retired' AND activated_at_ns IS NOT NULL AND retired_at_ns IS NOT NULL)
    )
);

CREATE UNIQUE INDEX IF NOT EXISTS situation_models_one_active_entity_idx
    ON situation_models(entity_type)
    WHERE status = 'active';

CREATE UNIQUE INDEX IF NOT EXISTS situation_models_digest_idx
    ON situation_models(digest);

-- ---------------------------------------------------------------------------
-- model_activations
-- [F-08] Every activation is auditable, including how live entities were treated.
-- ---------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS model_activations (
    id                    TEXT PRIMARY KEY,
    entity_type           TEXT NOT NULL,
    model_digest          TEXT NOT NULL,
    previous_digest       TEXT,
    mode                  TEXT NOT NULL CHECK (mode IN ('continue', 'reset')),
    entities_migrated     INTEGER NOT NULL DEFAULT 0,
    episodes_superseded   INTEGER NOT NULL DEFAULT 0,
    actor                 TEXT NOT NULL,
    activated_at_ns       INTEGER NOT NULL,
    FOREIGN KEY (model_digest) REFERENCES situation_models(digest),
    FOREIGN KEY (previous_digest) REFERENCES situation_models(digest),
    CHECK (model_digest <> COALESCE(previous_digest, '')),
    CHECK (entities_migrated >= 0),
    CHECK (episodes_superseded >= 0)
);

CREATE INDEX IF NOT EXISTS model_activations_entity_type_idx
    ON model_activations(entity_type, activated_at_ns);

-- ---------------------------------------------------------------------------
-- events
-- [F-14] V0's late_ignored/ignore_reason pair is generalized. Every well-formed
-- envelope is persisted; admission decides whether it feeds facts. A sensor that
-- starts emitting out-of-contract values stays visible as evidence instead of
-- becoming a counter.
-- ---------------------------------------------------------------------------
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
    -- [F-13] compared on duplicate id; a mismatch is 409 event_id_reuse, never a
    -- silent duplicate-accept.
    payload_hash      TEXT NOT NULL,
    model_digest      TEXT NOT NULL,
    admitted          INTEGER NOT NULL CHECK (admitted IN (0, 1)),
    admission_reason  TEXT NOT NULL CHECK (
        admission_reason IN (
            'admitted',
            'late_beyond_allowance',
            'value_out_of_contract'
        )
    ),
    created_at_ns     INTEGER NOT NULL,
    CHECK (
        (value_type = 'number' AND numeric_value IS NOT NULL AND text_value IS NULL)
        OR
        (value_type = 'string' AND numeric_value IS NULL AND text_value IS NOT NULL)
        OR
        (value_type = 'none'   AND numeric_value IS NULL AND text_value IS NULL)
    ),
    CHECK (
        (admitted = 1 AND admission_reason = 'admitted')
        OR
        (admitted = 0 AND admission_reason <> 'admitted')
    ),
    CHECK (arrival_time_ns >= event_time_ns),        -- [F-24]
    FOREIGN KEY (model_digest) REFERENCES situation_models(digest)
);

-- Aggregate path: window queries filter admitted = 1 and are bounded by the
-- window's max_samples. This partial index is the one the hot query must use.
CREATE INDEX IF NOT EXISTS events_window_idx
    ON events(entity_type, entity_id, type,
              event_time_ns DESC, arrival_time_ns DESC, id DESC)
    WHERE admitted = 1;

-- age facts and trace export order by arrival.
CREATE INDEX IF NOT EXISTS events_entity_arrival_idx
    ON events(entity_type, entity_id, arrival_time_ns);

-- Snapshot assembly reads recent events including non-admitted ones. [F-15]
CREATE INDEX IF NOT EXISTS events_entity_recent_idx
    ON events(entity_type, entity_id, event_time_ns DESC);

-- ---------------------------------------------------------------------------
-- entity_state
-- Mutable projection. Never a source of truth for history.
-- ---------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS entity_state (
    entity_type          TEXT NOT NULL,
    entity_id            TEXT NOT NULL,
    model_digest         TEXT NOT NULL,
    event_horizon_ns     INTEGER NOT NULL,
    last_evaluated_ns    INTEGER NOT NULL,
    current_state        TEXT NOT NULL,
    -- Evaluation instant at which current_state was entered; trigger wake source.
    state_entered_at_ns  INTEGER NOT NULL,
    situation_version    INTEGER NOT NULL DEFAULT 0 CHECK (situation_version >= 0),
    facts_json           TEXT NOT NULL CHECK (json_valid(facts_json)),
    -- Band index and quality class per fact; the material-change comparator. [F-07]
    fact_bands_json      TEXT NOT NULL CHECK (json_valid(fact_bands_json)),
    -- Pending sustained_for candidacies, so a restart cannot restart the clock. [F-10]
    candidacies_json     TEXT NOT NULL CHECK (json_valid(candidacies_json)),
    -- Exact next age-band, candidacy, or sustained-trigger boundary. [F-06]
    next_wake_ns         INTEGER,
    updated_at_ns        INTEGER NOT NULL,
    PRIMARY KEY (entity_type, entity_id),
    CHECK (state_entered_at_ns <= last_evaluated_ns),
    FOREIGN KEY (model_digest) REFERENCES situation_models(digest)
);

-- The live scheduler claims strictly overdue wakes. Recorded input at an exact
-- boundary is ordered first; replay --until performs the final inclusive drain.
CREATE INDEX IF NOT EXISTS entity_state_next_wake_idx
    ON entity_state(next_wake_ns)
    WHERE next_wake_ns IS NOT NULL;

-- ---------------------------------------------------------------------------
-- situations
-- Immutable material versions. [F-23] V0's foreign key to entity_state is removed:
-- immutable history must not depend on the existence of a mutable projection row.
-- ---------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS situations (
    entity_type          TEXT NOT NULL,
    entity_id            TEXT NOT NULL,
    version              INTEGER NOT NULL CHECK (version >= 1),
    state                TEXT NOT NULL,
    previous_state       TEXT NOT NULL,   -- cold start = the model's default state [F-11]
    event_horizon_ns     INTEGER NOT NULL,
    evaluation_instant_ns INTEGER NOT NULL,
    evaluation_basis     TEXT NOT NULL CHECK (
        evaluation_basis IN ('event', 'wake', 'model_migration')
    ),
    trigger_event_id     TEXT,
    model_digest         TEXT NOT NULL,
    runtime_version      TEXT NOT NULL,
    -- Why this version exists at all; drives the material-change counters.
    material_reasons_json TEXT NOT NULL CHECK (json_valid(material_reasons_json)),
    facts_json           TEXT NOT NULL CHECK (json_valid(facts_json)),
    fact_bands_json      TEXT NOT NULL CHECK (json_valid(fact_bands_json)),
    candidacies_json     TEXT NOT NULL CHECK (json_valid(candidacies_json)),
    -- Bounded provenance summaries; never an unbounded list of every sample.
    evidence_ids_json    TEXT NOT NULL CHECK (json_valid(evidence_ids_json)),
    evaluation_json      TEXT NOT NULL CHECK (json_valid(evaluation_json)),
    -- Every matched trigger with its outcome and reason, admitted or suppressed.
    -- Nothing that could have caused cognition disappears silently. [F-19]
    triggers_json        TEXT NOT NULL CHECK (json_valid(triggers_json)),
    canonical_hash       TEXT NOT NULL,
    created_at_ns        INTEGER NOT NULL,
    PRIMARY KEY (entity_type, entity_id, version),
    CHECK (length(canonical_hash) = 71 AND canonical_hash GLOB 'sha256:*'),
    FOREIGN KEY (model_digest) REFERENCES situation_models(digest),
    FOREIGN KEY (trigger_event_id) REFERENCES events(id)
);

CREATE INDEX IF NOT EXISTS situations_state_idx
    ON situations(entity_type, state, created_at_ns);

-- ---------------------------------------------------------------------------
-- episodes
-- ---------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS episodes (
    id                    TEXT PRIMARY KEY,
    entity_type           TEXT NOT NULL,
    entity_id             TEXT NOT NULL,
    situation_version     INTEGER NOT NULL,
    trigger_id            TEXT NOT NULL,
    episode_type          TEXT NOT NULL,
    status                TEXT NOT NULL CHECK (
        status IN ('pending', 'running', 'completed', 'failed',
                   'superseded_by_model_change')
    ),
    snapshot_json         TEXT NOT NULL CHECK (json_valid(snapshot_json)),
    snapshot_hash         TEXT NOT NULL,
    snapshot_truncated    INTEGER NOT NULL DEFAULT 0 CHECK (snapshot_truncated IN (0,1)),
    request_digest        TEXT,
    -- [F-19] cumulative known cost across attempts. A provider call abandoned by
    -- a process crash may be billed without returning usage; that uncertainty is
    -- reported separately and is not fabricated here.
    input_tokens_total    INTEGER NOT NULL DEFAULT 0 CHECK (input_tokens_total >= 0),
    output_tokens_total   INTEGER NOT NULL DEFAULT 0 CHECK (output_tokens_total >= 0),
    cost_micros_total     INTEGER NOT NULL DEFAULT 0 CHECK (cost_micros_total >= 0),
    duration_ms_total     INTEGER NOT NULL DEFAULT 0 CHECK (duration_ms_total >= 0),
    -- Five total attempts: initial plus at most four manual retries.
    attempt_count         INTEGER NOT NULL DEFAULT 0 CHECK (
        attempt_count BETWEEN 0 AND 5
    ),
    started_at_ns         INTEGER,
    finished_at_ns        INTEGER,
    error_code            TEXT,
    error_detail_redacted TEXT,
    -- Deterministic admission time: the triggering Situation's evaluation instant.
    admitted_at_ns        INTEGER NOT NULL,
    UNIQUE (entity_type, entity_id, situation_version, trigger_id),
    FOREIGN KEY (entity_type, entity_id, situation_version)
        REFERENCES situations(entity_type, entity_id, version)
);

CREATE INDEX IF NOT EXISTS episodes_status_admitted_idx
    ON episodes(status, admitted_at_ns);

-- Per-entity budget, episode-type cooldown, and process budget checks. [F-19]
CREATE INDEX IF NOT EXISTS episodes_entity_admission_idx
    ON episodes(entity_type, entity_id, admitted_at_ns);

CREATE INDEX IF NOT EXISTS episodes_entity_type_cooldown_idx
    ON episodes(entity_type, entity_id, episode_type, admitted_at_ns);

CREATE INDEX IF NOT EXISTS episodes_admission_idx
    ON episodes(admitted_at_ns);

-- ---------------------------------------------------------------------------
-- decisions
-- ---------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS decisions (
    id                     TEXT PRIMARY KEY,
    episode_id             TEXT NOT NULL UNIQUE,
    output_json            TEXT NOT NULL CHECK (json_valid(output_json)),
    output_schema_digest   TEXT NOT NULL,
    evidence_ids_json      TEXT NOT NULL CHECK (json_valid(evidence_ids_json)),
    -- Extracted via the model's confidence_pointer so the evaluation harness can
    -- measure overconfidence without parsing domain-specific JSON.
    confidence             REAL CHECK (confidence IS NULL OR (confidence >= 0 AND confidence <= 1)),
    reasoning_provider     TEXT NOT NULL,
    reasoning_model        TEXT NOT NULL,
    created_at_ns          INTEGER NOT NULL,
    FOREIGN KEY (episode_id) REFERENCES episodes(id)
);
