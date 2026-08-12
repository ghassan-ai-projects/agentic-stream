-- Logical SQLite schema baseline for the first implementation.
-- Migration files may split this by milestone, but must preserve these keys
-- and uniqueness constraints unless an ADR changes the contract.

PRAGMA foreign_keys = ON;

CREATE TABLE schema_migrations (
    version             INTEGER PRIMARY KEY,
    name                TEXT NOT NULL,
    applied_at          TEXT NOT NULL
) STRICT;

CREATE TABLE artifacts (
    artifact_id         TEXT PRIMARY KEY,
    sha256              BLOB NOT NULL UNIQUE CHECK (length(sha256) = 32),
    media_type          TEXT NOT NULL,
    size_bytes          INTEGER NOT NULL CHECK (size_bytes >= 0),
    classification      TEXT NOT NULL,
    storage_path        TEXT NOT NULL,
    created_at          TEXT NOT NULL,
    retain_until        TEXT,
    ref_count           INTEGER NOT NULL DEFAULT 0 CHECK (ref_count >= 0)
) STRICT;

CREATE TABLE spec_deployments (
    deployment_id       TEXT PRIMARY KEY,
    tenant_id           TEXT NOT NULL,
    spec_name           TEXT NOT NULL,
    spec_version        TEXT NOT NULL,
    spec_schema_version TEXT NOT NULL,
    spec_sha256         BLOB NOT NULL CHECK (length(spec_sha256) = 32),
    source_json          BLOB NOT NULL,
    compiled_ir         BLOB NOT NULL,
    status              TEXT NOT NULL CHECK (
        status IN ('staged', 'active', 'retired', 'rejected')
    ),
    activated_at        TEXT,
    created_at          TEXT NOT NULL,
    UNIQUE (tenant_id, spec_name, spec_version),
    UNIQUE (tenant_id, spec_sha256)
) STRICT;

CREATE UNIQUE INDEX one_active_spec_per_name
    ON spec_deployments(tenant_id, spec_name)
    WHERE status = 'active';

CREATE TABLE event_log (
    position            INTEGER PRIMARY KEY AUTOINCREMENT,
    tenant_id           TEXT NOT NULL,
    partition_id        INTEGER NOT NULL CHECK (partition_id >= 0),
    event_id            TEXT NOT NULL,
    event_type          TEXT NOT NULL,
    schema_version      TEXT NOT NULL,
    source              TEXT NOT NULL,
    partition_key       TEXT NOT NULL,
    entity_type         TEXT NOT NULL,
    entity_id           TEXT NOT NULL,
    event_time          TEXT NOT NULL,
    observed_at         TEXT,
    ingested_at         TEXT NOT NULL,
    correlation_id      TEXT,
    causation_id        TEXT,
    traceparent         TEXT,
    classification      TEXT NOT NULL,
    quality_json        BLOB NOT NULL,
    payload_json        BLOB NOT NULL,
    payload_sha256      BLOB NOT NULL CHECK (length(payload_sha256) = 32),
    original_artifact_id TEXT REFERENCES artifacts(artifact_id),
    created_at          TEXT NOT NULL,
    UNIQUE (tenant_id, event_id)
) STRICT;

CREATE INDEX event_log_partition_position
    ON event_log(tenant_id, partition_id, position);
CREATE INDEX event_log_entity_time
    ON event_log(tenant_id, entity_type, entity_id, event_time);
CREATE INDEX event_log_type_time
    ON event_log(tenant_id, event_type, event_time);

CREATE TABLE event_inbox (
    consumer_name       TEXT NOT NULL,
    tenant_id           TEXT NOT NULL,
    event_id            TEXT NOT NULL,
    log_position        INTEGER NOT NULL REFERENCES event_log(position),
    applied_at          TEXT NOT NULL,
    PRIMARY KEY (consumer_name, tenant_id, event_id)
) STRICT;

CREATE TABLE connector_checkpoints (
    connector_id        TEXT PRIMARY KEY,
    connector_kind      TEXT NOT NULL,
    checkpoint_version  INTEGER NOT NULL CHECK (checkpoint_version >= 1),
    checkpoint_blob     BLOB NOT NULL,
    updated_at          TEXT NOT NULL
) STRICT;

CREATE TABLE partition_checkpoints (
    consumer_name       TEXT NOT NULL,
    tenant_id           TEXT NOT NULL,
    partition_id        INTEGER NOT NULL CHECK (partition_id >= 0),
    last_position       INTEGER NOT NULL DEFAULT 0 CHECK (last_position >= 0),
    watermark           TEXT,
    max_event_time      TEXT,
    updated_at          TEXT NOT NULL,
    PRIMARY KEY (consumer_name, tenant_id, partition_id)
) STRICT;

CREATE TABLE operator_state (
    deployment_id       TEXT NOT NULL REFERENCES spec_deployments(deployment_id),
    tenant_id           TEXT NOT NULL,
    partition_id        INTEGER NOT NULL CHECK (partition_id >= 0),
    operator_id         TEXT NOT NULL,
    state_key           TEXT NOT NULL,
    state_version       INTEGER NOT NULL CHECK (state_version >= 1),
    codec_version       INTEGER NOT NULL CHECK (codec_version >= 1),
    state_blob          BLOB NOT NULL,
    state_sha256        BLOB NOT NULL CHECK (length(state_sha256) = 32),
    updated_at          TEXT NOT NULL,
    PRIMARY KEY (
        deployment_id,
        tenant_id,
        partition_id,
        operator_id,
        state_key
    )
) STRICT;

CREATE TABLE timers (
    timer_id            TEXT PRIMARY KEY,
    deployment_id       TEXT NOT NULL REFERENCES spec_deployments(deployment_id),
    tenant_id           TEXT NOT NULL,
    partition_id        INTEGER NOT NULL CHECK (partition_id >= 0),
    operator_id         TEXT NOT NULL,
    state_key           TEXT NOT NULL,
    timer_kind          TEXT NOT NULL CHECK (
        timer_kind IN ('event_time', 'processing_time')
    ),
    due_at              TEXT NOT NULL,
    payload_json        BLOB NOT NULL,
    status              TEXT NOT NULL CHECK (
        status IN ('pending', 'fired', 'cancelled')
    ),
    created_at          TEXT NOT NULL,
    fired_at            TEXT,
    UNIQUE (
        deployment_id,
        tenant_id,
        partition_id,
        operator_id,
        state_key,
        timer_kind,
        due_at
    )
) STRICT;

CREATE INDEX timers_due
    ON timers(timer_kind, status, due_at, partition_id);

CREATE TABLE lineage_sets (
    lineage_id          TEXT PRIMARY KEY,
    sha256              BLOB NOT NULL UNIQUE CHECK (length(sha256) = 32),
    reference_count     INTEGER NOT NULL CHECK (reference_count >= 1),
    references_json     BLOB NOT NULL,
    created_at          TEXT NOT NULL
) STRICT;

CREATE TABLE situations (
    situation_id        TEXT PRIMARY KEY,
    tenant_id           TEXT NOT NULL,
    deployment_id       TEXT NOT NULL REFERENCES spec_deployments(deployment_id),
    situation_type      TEXT NOT NULL,
    entity_type         TEXT NOT NULL,
    entity_id           TEXT NOT NULL,
    partition_id        INTEGER NOT NULL CHECK (partition_id >= 0),
    occurrence_id       TEXT NOT NULL,
    current_version     INTEGER NOT NULL CHECK (current_version >= 1),
    phase               TEXT NOT NULL,
    status              TEXT NOT NULL,
    first_event_time    TEXT NOT NULL,
    latest_event_time   TEXT NOT NULL,
    updated_at          TEXT NOT NULL,
    created_at          TEXT NOT NULL,
    state_codec_version INTEGER NOT NULL CHECK (state_codec_version >= 1),
    state_json          BLOB NOT NULL,
    state_sha256        BLOB NOT NULL CHECK (length(state_sha256) = 32),
    UNIQUE (
        tenant_id,
        situation_type,
        entity_type,
        entity_id,
        occurrence_id
    )
) STRICT;

CREATE INDEX situations_entity_status
    ON situations(tenant_id, entity_type, entity_id, status);
CREATE INDEX situations_type_phase
    ON situations(tenant_id, situation_type, phase, updated_at);

CREATE TABLE situation_versions (
    situation_id        TEXT NOT NULL REFERENCES situations(situation_id),
    version             INTEGER NOT NULL CHECK (version >= 1),
    previous_version    INTEGER,
    phase               TEXT NOT NULL,
    previous_phase      TEXT,
    severity            INTEGER NOT NULL CHECK (severity BETWEEN 0 AND 100),
    confidence          REAL NOT NULL CHECK (confidence BETWEEN 0 AND 1),
    completeness        TEXT NOT NULL CHECK (
        completeness IN (
            'provisional',
            'on_time',
            'corrected',
            'final_by_policy',
            'uncertain'
        )
    ),
    event_horizon       TEXT NOT NULL,
    watermark           TEXT,
    valid_from          TEXT NOT NULL,
    valid_until         TEXT,
    snapshot_json       BLOB NOT NULL,
    snapshot_sha256     BLOB NOT NULL CHECK (length(snapshot_sha256) = 32),
    lineage_id          TEXT NOT NULL REFERENCES lineage_sets(lineage_id),
    created_at          TEXT NOT NULL,
    PRIMARY KEY (situation_id, version),
    CHECK (
        previous_version IS NULL OR
        (previous_version >= 1 AND previous_version < version)
    )
) STRICT;

CREATE TABLE trigger_evaluations (
    trigger_id          TEXT PRIMARY KEY,
    tenant_id           TEXT NOT NULL,
    deployment_id       TEXT NOT NULL REFERENCES spec_deployments(deployment_id),
    trigger_name        TEXT NOT NULL,
    situation_id        TEXT NOT NULL,
    situation_version   INTEGER NOT NULL,
    score               REAL NOT NULL,
    threshold           REAL NOT NULL,
    lane                TEXT NOT NULL CHECK (lane IN ('fast', 'deep')),
    outcome             TEXT NOT NULL CHECK (
        outcome IN (
            'ignored',
            'debounced',
            'coalesced',
            'deferred',
            'admitted',
            'superseded',
            'expired',
            'rejected'
        )
    ),
    reasons_json        BLOB NOT NULL,
    policy_sha256       BLOB NOT NULL CHECK (length(policy_sha256) = 32),
    evaluated_at        TEXT NOT NULL,
    FOREIGN KEY (situation_id, situation_version)
        REFERENCES situation_versions(situation_id, version)
) STRICT;

CREATE INDEX trigger_evaluations_situation
    ON trigger_evaluations(situation_id, situation_version, evaluated_at);

CREATE TABLE scheduler_items (
    scheduler_item_id   TEXT PRIMARY KEY,
    trigger_id          TEXT NOT NULL UNIQUE REFERENCES trigger_evaluations(trigger_id),
    tenant_id           TEXT NOT NULL,
    situation_id        TEXT NOT NULL,
    situation_version   INTEGER NOT NULL,
    lane                TEXT NOT NULL CHECK (lane IN ('fast', 'deep')),
    priority            REAL NOT NULL,
    status              TEXT NOT NULL CHECK (
        status IN (
            'pending',
            'admitted',
            'coalesced',
            'expired',
            'cancelled',
            'completed'
        )
    ),
    dedupe_key          BLOB NOT NULL UNIQUE CHECK (length(dedupe_key) = 32),
    not_before          TEXT,
    expires_at          TEXT NOT NULL,
    created_at          TEXT NOT NULL,
    updated_at          TEXT NOT NULL,
    FOREIGN KEY (situation_id, situation_version)
        REFERENCES situation_versions(situation_id, version)
) STRICT;

CREATE INDEX scheduler_pending
    ON scheduler_items(status, lane, priority DESC, expires_at, created_at);

CREATE TABLE episodes (
    episode_id          TEXT PRIMARY KEY,
    scheduler_item_id   TEXT NOT NULL UNIQUE REFERENCES scheduler_items(scheduler_item_id),
    tenant_id           TEXT NOT NULL,
    situation_id        TEXT NOT NULL,
    situation_version   INTEGER NOT NULL,
    executor_name       TEXT NOT NULL,
    executor_version    TEXT NOT NULL,
    model_policy        TEXT NOT NULL,
    prompt_version      TEXT NOT NULL,
    snapshot_sha256     BLOB NOT NULL CHECK (length(snapshot_sha256) = 32),
    admission_key       BLOB NOT NULL UNIQUE CHECK (length(admission_key) = 32),
    request_json        BLOB NOT NULL,
    status              TEXT NOT NULL CHECK (
        status IN (
            'accepted',
            'queued',
            'running',
            'cancelling',
            'decided',
            'no_action',
            'needs_human',
            'superseded',
            'timed_out',
            'budget_exhausted',
            'failed',
            'cancelled',
            'interrupted'
        )
    ),
    accepted_at         TEXT NOT NULL,
    started_at          TEXT,
    ended_at            TEXT,
    terminal_json       BLOB,
    FOREIGN KEY (situation_id, situation_version)
        REFERENCES situation_versions(situation_id, version)
) STRICT;

CREATE TABLE runtime_owner (
    singleton_id   INTEGER PRIMARY KEY CHECK (singleton_id = 1),
    owner_epoch    TEXT NOT NULL,
    owner_instance TEXT NOT NULL,
    acquired_at    TEXT NOT NULL,
    heartbeat_at   TEXT NOT NULL,
    lease_until    TEXT NOT NULL
) STRICT;

CREATE INDEX episodes_situation_status
    ON episodes(situation_id, status, accepted_at);

CREATE UNIQUE INDEX one_live_episode_per_situation
    ON episodes(situation_id)
    WHERE status IN ('accepted', 'queued', 'running', 'cancelling');

CREATE TABLE episode_events (
    episode_id          TEXT NOT NULL REFERENCES episodes(episode_id),
    sequence            INTEGER NOT NULL CHECK (sequence >= 1),
    event_type          TEXT NOT NULL,
    event_json          BLOB NOT NULL,
    event_sha256        BLOB NOT NULL CHECK (length(event_sha256) = 32),
    durable             INTEGER NOT NULL CHECK (durable IN (0, 1)),
    occurred_at         TEXT NOT NULL,
    PRIMARY KEY (episode_id, sequence)
) STRICT;

CREATE TABLE decisions (
    decision_id         TEXT PRIMARY KEY,
    episode_id          TEXT NOT NULL REFERENCES episodes(episode_id),
    ordinal             INTEGER NOT NULL CHECK (ordinal >= 1),
    situation_id        TEXT NOT NULL,
    situation_version   INTEGER NOT NULL,
    raw_json            BLOB NOT NULL,
    decision_sha256     BLOB NOT NULL CHECK (length(decision_sha256) = 32),
    validation_status   TEXT NOT NULL CHECK (
        validation_status IN ('proposed', 'accepted', 'rejected')
    ),
    validation_json     BLOB NOT NULL,
    created_at          TEXT NOT NULL,
    UNIQUE (episode_id, ordinal),
    FOREIGN KEY (situation_id, situation_version)
        REFERENCES situation_versions(situation_id, version)
) STRICT;

CREATE TABLE intents (
    intent_id           TEXT PRIMARY KEY,
    decision_id         TEXT NOT NULL REFERENCES decisions(decision_id),
    tenant_id           TEXT NOT NULL,
    situation_id        TEXT NOT NULL,
    situation_version   INTEGER NOT NULL,
    intent_type         TEXT NOT NULL,
    risk_class          TEXT NOT NULL CHECK (
        risk_class IN ('R0', 'R1', 'R2', 'R3', 'R4')
    ),
    intent_json         BLOB NOT NULL,
    intent_sha256       BLOB NOT NULL CHECK (length(intent_sha256) = 32),
    expires_at          TEXT NOT NULL,
    policy_status       TEXT NOT NULL CHECK (
        policy_status IN (
            'pending',
            'approved',
            'approval_required',
            'simulated',
            'denied',
            'expired',
            'stale'
        )
    ),
    created_at          TEXT NOT NULL,
    updated_at          TEXT NOT NULL,
    FOREIGN KEY (situation_id, situation_version)
        REFERENCES situation_versions(situation_id, version)
) STRICT;

CREATE TABLE approvals (
    approval_id         TEXT PRIMARY KEY,
    intent_id           TEXT NOT NULL REFERENCES intents(intent_id),
    status              TEXT NOT NULL CHECK (
        status IN ('pending', 'approved', 'denied', 'expired')
    ),
    requested_at        TEXT NOT NULL,
    expires_at          TEXT NOT NULL,
    decided_at          TEXT,
    approver_identity   TEXT,
    reason              TEXT,
    approval_json       BLOB NOT NULL
) STRICT;

CREATE UNIQUE INDEX one_pending_approval_per_intent
    ON approvals(intent_id)
    WHERE status = 'pending';

CREATE TABLE commands (
    command_id          TEXT PRIMARY KEY,
    intent_id           TEXT NOT NULL UNIQUE REFERENCES intents(intent_id),
    tenant_id           TEXT NOT NULL,
    effector_route      TEXT NOT NULL,
    normalized_target   TEXT NOT NULL,
    idempotency_key     BLOB NOT NULL UNIQUE CHECK (length(idempotency_key) = 32),
    command_json        BLOB NOT NULL,
    command_sha256      BLOB NOT NULL CHECK (length(command_sha256) = 32),
    status              TEXT NOT NULL CHECK (
        status IN (
            'pending',
            'dispatching',
            'succeeded',
            'failed',
            'outcome_unknown',
            'reconciling',
            'manual_review'
        )
    ),
    created_at          TEXT NOT NULL,
    updated_at          TEXT NOT NULL
) STRICT;

CREATE TABLE outbox (
    outbox_id           INTEGER PRIMARY KEY AUTOINCREMENT,
    kind                TEXT NOT NULL,
    aggregate_id        TEXT NOT NULL,
    aggregate_version   INTEGER NOT NULL CHECK (aggregate_version >= 1),
    payload_json        BLOB NOT NULL,
    status              TEXT NOT NULL CHECK (
        status IN ('pending', 'leased', 'delivered', 'failed')
    ),
    attempt_count       INTEGER NOT NULL DEFAULT 0 CHECK (attempt_count >= 0),
    available_at        TEXT NOT NULL,
    lease_owner         TEXT,
    lease_until         TEXT,
    last_error_code     TEXT,
    created_at          TEXT NOT NULL,
    delivered_at        TEXT,
    UNIQUE (kind, aggregate_id, aggregate_version)
) STRICT;

CREATE INDEX outbox_dispatch
    ON outbox(status, available_at, lease_until, outbox_id);

CREATE TABLE outcomes (
    outcome_id          TEXT PRIMARY KEY,
    command_id          TEXT NOT NULL REFERENCES commands(command_id),
    ordinal             INTEGER NOT NULL CHECK (ordinal >= 1),
    status              TEXT NOT NULL,
    provider_result_json BLOB,
    observed_effect_json BLOB,
    reconciliation_status TEXT,
    outcome_sha256      BLOB NOT NULL CHECK (length(outcome_sha256) = 32),
    occurred_at         TEXT NOT NULL,
    UNIQUE (command_id, ordinal)
) STRICT;

CREATE TABLE replay_jobs (
    replay_id           TEXT PRIMARY KEY,
    tenant_id           TEXT NOT NULL,
    mode                TEXT NOT NULL CHECK (
        mode IN ('deterministic', 'recorded', 'shadow', 'counterfactual')
    ),
    source_from         TEXT NOT NULL,
    source_until        TEXT NOT NULL,
    deployment_id       TEXT NOT NULL REFERENCES spec_deployments(deployment_id),
    executor_name       TEXT,
    status              TEXT NOT NULL CHECK (
        status IN ('pending', 'running', 'completed', 'failed', 'cancelled')
    ),
    isolated_database   TEXT NOT NULL,
    result_artifact_id  TEXT REFERENCES artifacts(artifact_id),
    created_at          TEXT NOT NULL,
    started_at          TEXT,
    ended_at            TEXT
) STRICT;

CREATE TABLE evidence_call_ledger (
    tenant_id             TEXT NOT NULL,
    episode_id            TEXT NOT NULL,
    attempt_id            TEXT NOT NULL,
    fence                 INTEGER NOT NULL CHECK (fence >= 1),
    call_id               TEXT NOT NULL,
    token_id              TEXT NOT NULL,
    runtime_epoch         TEXT NOT NULL,
    tool_name             TEXT NOT NULL,
    situation_id          TEXT NOT NULL,
    situation_version     INTEGER NOT NULL CHECK (situation_version >= 1),
    entity_id             TEXT NOT NULL,
    time_from             TEXT NOT NULL,
    time_until            TEXT NOT NULL,
    max_rows              INTEGER NOT NULL CHECK (max_rows >= 1),
    max_bytes             INTEGER NOT NULL CHECK (max_bytes >= 1),
    deadline              TEXT NOT NULL,
    request_sha256        BLOB NOT NULL CHECK (length(request_sha256) = 32),
    status                TEXT NOT NULL CHECK (status IN ('running', 'completed', 'failed', 'interrupted')),
    result_json           BLOB,
    result_sha256         BLOB CHECK (result_sha256 IS NULL OR length(result_sha256) = 32),
    result_bytes          INTEGER CHECK (result_bytes IS NULL OR result_bytes = length(result_json)),
    row_count             INTEGER CHECK (row_count IS NULL OR row_count >= 0),
    error_code            TEXT,
    lease_owner           TEXT NOT NULL,
    lease_until           TEXT NOT NULL,
    reserved_at           TEXT NOT NULL,
    completed_at          TEXT,
    PRIMARY KEY (tenant_id, episode_id, attempt_id, fence, call_id),
    UNIQUE (attempt_id, fence, call_id),
    FOREIGN KEY (attempt_id, episode_id, fence)
        REFERENCES episode_attempts(attempt_id, episode_id, fence)
) STRICT;

CREATE TABLE runtime_owner (
    singleton_id   INTEGER PRIMARY KEY CHECK (singleton_id = 1),
    owner_epoch    TEXT NOT NULL,
    owner_instance TEXT NOT NULL,
    acquired_at    TEXT NOT NULL,
    heartbeat_at   TEXT NOT NULL,
    lease_until    TEXT NOT NULL
) STRICT;
