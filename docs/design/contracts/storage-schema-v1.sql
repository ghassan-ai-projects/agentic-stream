-- Logical SQLite schema snapshot for the current v1 implementation.
--
-- This contract is the cumulative state after migrations 001-030. The numbered
-- migration files remain the upgrade history and executable source of truth;
-- keep this snapshot synchronized whenever a migration changes the schema.

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
    tracestate          TEXT,
    UNIQUE (tenant_id, event_id)
) STRICT;

CREATE INDEX event_log_partition_position
    ON event_log(tenant_id, partition_id, position);
CREATE INDEX event_log_entity_time
    ON event_log(tenant_id, entity_type, entity_id, event_time);
CREATE INDEX event_log_type_time
    ON event_log(tenant_id, event_type, event_time);

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
    redriven_at     TEXT,
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

CREATE TABLE watch_conditions (
    watch_id             TEXT PRIMARY KEY,
    tenant_id            TEXT NOT NULL,
    situation_id         TEXT NOT NULL,
    situation_version    INTEGER NOT NULL CHECK (situation_version >= 1),
    expression           TEXT NOT NULL,
    target               TEXT NOT NULL,
    expires_at           TEXT NOT NULL,
    remaining_fires      INTEGER NOT NULL CHECK (remaining_fires BETWEEN 0 AND 100),
    status               TEXT NOT NULL CHECK (status IN ('active', 'expired', 'disabled')),
    created_at           TEXT NOT NULL,
    updated_at           TEXT NOT NULL,
    max_fires            INTEGER NOT NULL DEFAULT 1 CHECK (max_fires BETWEEN 1 AND 100)
) STRICT;

CREATE TABLE watch_fires (
    watch_id             TEXT NOT NULL REFERENCES watch_conditions(watch_id),
    event_id             TEXT NOT NULL,
    fired_at             TEXT NOT NULL,
    PRIMARY KEY (watch_id, event_id)
) STRICT;

CREATE INDEX watch_conditions_due
    ON watch_conditions(tenant_id, status, expires_at);

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
    last_reasoned_version INTEGER NOT NULL DEFAULT 0 CHECK (last_reasoned_version >= 0),
    phase               TEXT NOT NULL,
    status              TEXT NOT NULL,
    first_event_time    TEXT NOT NULL,
    latest_event_time   TEXT NOT NULL,
    updated_at          TEXT NOT NULL,
    created_at          TEXT NOT NULL,
    state_codec_version INTEGER NOT NULL DEFAULT 0 CHECK (state_codec_version >= 0),
    state_json          BLOB NOT NULL DEFAULT X'7B7D',
    state_sha256        BLOB NOT NULL DEFAULT X'0000000000000000000000000000000000000000000000000000000000000000' CHECK (length(state_sha256) = 32),
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
    traceparent         TEXT,
    tracestate          TEXT,
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
    delta_json          BLOB NOT NULL DEFAULT X'7B7D',
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
    kind                TEXT NOT NULL DEFAULT 'standard' CHECK (kind IN ('standard', 'reconsider')),
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
    lifecycle_status    TEXT NOT NULL CHECK (
        lifecycle_status IN (
            'admitted',
            'running',
            'concluded',
            'closed',
            'superseded',
            'expired',
            'abandoned'
        )
    ),
    current_attempt_id  TEXT,
    current_fence       INTEGER NOT NULL DEFAULT 0 CHECK (current_fence >= 0),
    accepted_at         TEXT NOT NULL,
    started_at          TEXT,
    ended_at            TEXT,
    terminal_json       BLOB,
    prompt_sha256       BLOB CHECK (prompt_sha256 IS NULL OR length(prompt_sha256) = 32),
    objective_sha256    BLOB CHECK (objective_sha256 IS NULL OR length(objective_sha256) = 32),
    dispatch_policy     TEXT NOT NULL DEFAULT 'shadow' CHECK (dispatch_policy IN ('active', 'shadow')),
    policy_epoch        TEXT NOT NULL DEFAULT '',
    stale_rebind_count  INTEGER NOT NULL DEFAULT 0,
    FOREIGN KEY (situation_id, situation_version)
        REFERENCES situation_versions(situation_id, version)
) STRICT;

CREATE INDEX episodes_situation_lifecycle
    ON episodes(situation_id, lifecycle_status, accepted_at);

CREATE UNIQUE INDEX one_live_episode_per_situation
    ON episodes(situation_id)
    WHERE lifecycle_status IN ('admitted', 'running');

CREATE TABLE episode_events (
    episode_id          TEXT NOT NULL REFERENCES episodes(episode_id),
    sequence            INTEGER NOT NULL CHECK (sequence >= 1),
    attempt_id          TEXT,
    fence               INTEGER CHECK (fence IS NULL OR fence >= 1),
    event_type          TEXT NOT NULL,
    event_json          BLOB NOT NULL,
    event_sha256        BLOB NOT NULL CHECK (length(event_sha256) = 32),
    durable             INTEGER NOT NULL CHECK (durable IN (0, 1)),
    occurred_at         TEXT NOT NULL,
    PRIMARY KEY (episode_id, sequence)
) STRICT;

CREATE TABLE episode_rejections (
    rejection_id  TEXT PRIMARY KEY,
    episode_id    TEXT,
    attempt_id    TEXT,
    fence         INTEGER NOT NULL CHECK (fence >= 0),
    reason        TEXT NOT NULL CHECK (
        reason IN (
            'unknown_episode',
            'stale_attempt',
            'wrong_attempt',
            'terminal_attempt',
            'episode_closed',
            'schema_invalid',
            'snapshot_mismatch',
            'evidence_not_visible',
            'forged_reference',
            'oversized',
            'expired',
            'intent_type_not_allowed',
            'risk_ceiling_exceeded',
            'catalog_missing',
            'catalog_forged',
            'intent_type_not_in_catalog',
            'risk_label_mismatch',
            'parameter_schema_violation',
            'preset_mismatch',
            'ungrounded_evidence'
        )
    ),
    details_json  BLOB NOT NULL,
    created_at    TEXT NOT NULL,
    FOREIGN KEY (episode_id) REFERENCES episodes(episode_id)
) STRICT;

CREATE INDEX episode_rejections_identity
    ON episode_rejections(episode_id, attempt_id, fence);

CREATE TABLE episode_attempts (
    attempt_id             TEXT PRIMARY KEY,
    episode_id             TEXT NOT NULL REFERENCES episodes(episode_id),
    fence                  INTEGER NOT NULL CHECK (fence >= 1),
    status                 TEXT NOT NULL CHECK (
        status IN (
            'dispatched',
            'running',
            'cancelling',
            'produced',
            'declined',
            'cancelled',
            'failed',
            'timed_out',
            'abandoned'
        )
    ),
    started_at             TEXT,
    ended_at               TEXT,
    terminal_json          BLOB,
    artifact_manifest_json BLOB NOT NULL DEFAULT X'7B7D',
    owner_epoch            TEXT,
    UNIQUE (episode_id, fence)
) STRICT;

CREATE INDEX episode_attempts_live
    ON episode_attempts(episode_id, status, fence DESC);

CREATE UNIQUE INDEX episode_attempt_identity
    ON episode_attempts(attempt_id, episode_id, fence);

CREATE TABLE decisions (
    decision_id         TEXT PRIMARY KEY,
    episode_id          TEXT NOT NULL REFERENCES episodes(episode_id),
    attempt_id          TEXT,
    fence               INTEGER CHECK (fence IS NULL OR fence >= 1),
    ordinal             INTEGER NOT NULL CHECK (ordinal >= 1),
    situation_id        TEXT NOT NULL,
    situation_version   INTEGER NOT NULL,
    raw_json            BLOB NOT NULL,
    decision_sha256     BLOB NOT NULL CHECK (length(decision_sha256) = 32),
    validation_status   TEXT NOT NULL CHECK (
        validation_status IN ('proposed', 'accepted', 'rejected')
    ),
    rejection_reason    TEXT,
    validation_json     BLOB NOT NULL,
    created_at          TEXT NOT NULL,
    traceparent         TEXT,
    tracestate          TEXT,
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
    rate_limit_per_hour INTEGER NOT NULL DEFAULT 0,
    requires_approval   INTEGER NOT NULL DEFAULT 0,
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
    approval_json       BLOB NOT NULL,
    relay_identity      TEXT,
    nonce               TEXT,
    assertion_sha256    BLOB,
    withdrawn_at        TEXT,
    withdrawal_reason   TEXT
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
    traceparent         TEXT,
    tracestate          TEXT,
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
        REFERENCES episode_attempts(attempt_id, episode_id, fence),
    CHECK (status != 'completed' OR (result_json IS NOT NULL AND result_sha256 IS NOT NULL AND result_bytes IS NOT NULL AND row_count IS NOT NULL AND completed_at IS NOT NULL)),
    CHECK (status = 'running' OR result_json IS NOT NULL OR result_sha256 IS NULL)
) STRICT;

CREATE INDEX evidence_call_ledger_leases
    ON evidence_call_ledger(status, lease_until);

CREATE INDEX evidence_call_ledger_attempt
    ON evidence_call_ledger(tenant_id, episode_id, attempt_id, fence);

CREATE TABLE runtime_owner (
    singleton_id   INTEGER PRIMARY KEY CHECK (singleton_id = 1),
    owner_epoch    TEXT NOT NULL,
    owner_instance TEXT NOT NULL,
    acquired_at    TEXT NOT NULL,
    heartbeat_at   TEXT NOT NULL,
    lease_until    TEXT NOT NULL
) STRICT;

CREATE TABLE principals (
    principal_id TEXT PRIMARY KEY,
    tenant_id    TEXT NOT NULL,
    status       TEXT NOT NULL CHECK (status IN ('active', 'disabled')),
    created_at   TEXT NOT NULL,
    public_key   BLOB
) STRICT;

CREATE TABLE roles (
    role_id   TEXT PRIMARY KEY,
    role_name TEXT NOT NULL UNIQUE
) STRICT;

CREATE TABLE principal_roles (
    principal_id TEXT NOT NULL REFERENCES principals(principal_id),
    role_id      TEXT NOT NULL REFERENCES roles(role_id),
    PRIMARY KEY (principal_id, role_id)
) STRICT;

CREATE TABLE approval_authorities (
    tenant_id  TEXT NOT NULL,
    entity_id  TEXT NOT NULL,
    risk_class TEXT NOT NULL CHECK (risk_class IN ('R0', 'R1', 'R2', 'R3', 'R4')),
    role_id    TEXT NOT NULL REFERENCES roles(role_id),
    PRIMARY KEY (tenant_id, entity_id, risk_class, role_id)
) STRICT;

CREATE TABLE runtime_interlock (
    singleton_id INTEGER PRIMARY KEY CHECK (singleton_id = 1),
    status       TEXT NOT NULL CHECK (status IN ('ready', 'tripped')),
    reason       TEXT NOT NULL,
    version      INTEGER NOT NULL CHECK (version >= 1),
    updated_at   TEXT NOT NULL
) STRICT;

CREATE TABLE cost_limits (
    scope_key       TEXT PRIMARY KEY,
    tenant_id       TEXT,
    max_micro       INTEGER NOT NULL CHECK (max_micro >= 0),
    reserved_micro  INTEGER NOT NULL DEFAULT 0 CHECK (reserved_micro >= 0),
    spent_micro     INTEGER NOT NULL DEFAULT 0 CHECK (spent_micro >= 0),
    kill_switch     INTEGER NOT NULL DEFAULT 0 CHECK (kill_switch IN (0, 1)),
    updated_at      TEXT NOT NULL
) STRICT;

CREATE TABLE cost_reservations (
    reservation_id TEXT PRIMARY KEY,
    episode_id     TEXT NOT NULL UNIQUE,
    tenant_id      TEXT NOT NULL,
    reserved_micro INTEGER NOT NULL CHECK (reserved_micro >= 0),
    actual_micro   INTEGER NOT NULL DEFAULT 0 CHECK (actual_micro >= 0),
    status         TEXT NOT NULL CHECK (status IN ('reserved', 'settled', 'rejected')),
    created_at     TEXT NOT NULL,
    settled_at     TEXT
) STRICT;

CREATE TABLE policy_evaluations (
    evaluation_id          TEXT PRIMARY KEY,
    intent_id              TEXT NOT NULL REFERENCES intents(intent_id),
    decision_id            TEXT NOT NULL REFERENCES decisions(decision_id),
    policy_version         TEXT NOT NULL,
    policy_digest          TEXT NOT NULL,
    intent_sha256          BLOB NOT NULL CHECK (length(intent_sha256) = 32),
    decision_sha256        BLOB NOT NULL CHECK (length(decision_sha256) = 32),
    command_id             TEXT REFERENCES commands(command_id),
    approval_id            TEXT REFERENCES approvals(approval_id),
    result                 TEXT NOT NULL CHECK (
        result IN ('approved', 'approval_required', 'simulated', 'denied', 'stale', 'expired')
    ),
    reason                 TEXT NOT NULL,
    situation_version      INTEGER NOT NULL,
    evaluated_at           TEXT NOT NULL
) STRICT;

CREATE INDEX policy_evaluations_intent_time
    ON policy_evaluations(intent_id, evaluated_at DESC);

CREATE TABLE reconsiderations (
    reconsideration_id        TEXT PRIMARY KEY,
    tenant_id                 TEXT NOT NULL,
    situation_id              TEXT NOT NULL,
    superseded_version        INTEGER NOT NULL CHECK (superseded_version >= 1),
    correction_version        INTEGER NOT NULL CHECK (correction_version > superseded_version),
    correction_snapshot_sha256 BLOB NOT NULL CHECK (length(correction_snapshot_sha256) = 32),
    invalidated_command_id    TEXT NOT NULL REFERENCES commands(command_id),
    invalidated_outcome_id    TEXT NOT NULL REFERENCES outcomes(outcome_id),
    invalidated_outcome_sha256 BLOB NOT NULL CHECK (length(invalidated_outcome_sha256) = 32),
    trigger_id                TEXT UNIQUE REFERENCES trigger_evaluations(trigger_id),
    scheduler_item_id         TEXT UNIQUE REFERENCES scheduler_items(scheduler_item_id),
    created_at                TEXT NOT NULL,
    UNIQUE (situation_id, superseded_version, invalidated_command_id)
) STRICT;

CREATE INDEX reconsiderations_situation
    ON reconsiderations(situation_id, correction_version, created_at);

CREATE TABLE verifications (
    verification_id              TEXT PRIMARY KEY,
    intent_id                    TEXT NOT NULL UNIQUE REFERENCES intents(intent_id),
    command_id                   TEXT REFERENCES commands(command_id),
    outcome_id                   TEXT REFERENCES outcomes(outcome_id),
    status                       TEXT NOT NULL CHECK (
        status IN (
            'awaiting',
            'observed',
            'reconciled',
            'verified',
            'refuted',
            'inconclusive',
            'superseded_before_verification'
        )
    ),
    verdict_json                 BLOB,
    reconciled_at                TEXT,
    updated_at                   TEXT NOT NULL
) STRICT;

CREATE TABLE notifications (
    tenant_id       TEXT NOT NULL,
    cursor          INTEGER NOT NULL CHECK (cursor >= 1),
    event_id        TEXT NOT NULL,
    event_type      TEXT NOT NULL,
    event_json      BLOB NOT NULL,
    event_sha256    BLOB NOT NULL CHECK (length(event_sha256) = 32),
    created_at      TEXT NOT NULL,
    traceparent     TEXT,
    tracestate      TEXT,
    PRIMARY KEY (tenant_id, cursor),
    UNIQUE (tenant_id, event_id)
) STRICT;

CREATE TABLE notification_cursors (
    tenant_id       TEXT PRIMARY KEY,
    next_cursor     INTEGER NOT NULL CHECK (next_cursor >= 1)
) STRICT;

CREATE TABLE notification_event_tombstones (
    tenant_id       TEXT NOT NULL,
    event_id        TEXT NOT NULL,
    event_sha256    BLOB NOT NULL CHECK (length(event_sha256) = 32),
    retired_at      TEXT NOT NULL,
    PRIMARY KEY (tenant_id, event_id)
) STRICT;

CREATE TABLE notification_audits (
    audit_id         TEXT PRIMARY KEY,
    tenant_id        TEXT NOT NULL,
    action           TEXT NOT NULL CHECK (action IN ('cursor_expired', 'subscriber_skipped', 'subscriber_too_slow')),
    requested_cursor INTEGER,
    oldest_cursor    INTEGER,
    details_json     BLOB NOT NULL,
    created_at       TEXT NOT NULL
) STRICT;

CREATE TABLE notification_poison_attempts (
    tenant_id       TEXT NOT NULL,
    cursor          INTEGER NOT NULL CHECK (cursor >= 1),
    attempts        INTEGER NOT NULL CHECK (attempts >= 1),
    last_attempt_at TEXT NOT NULL,
    PRIMARY KEY (tenant_id, cursor),
    FOREIGN KEY (tenant_id, cursor) REFERENCES notifications(tenant_id, cursor)
) STRICT;

CREATE INDEX notifications_retention
    ON notifications(tenant_id, created_at, cursor);

CREATE TABLE intent_dispatch_counts (
    tenant_id       TEXT NOT NULL,
    intent_type     TEXT NOT NULL,
    bucket          TEXT NOT NULL,
    count           INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (tenant_id, intent_type, bucket)
);

CREATE UNIQUE INDEX cost_limits_tenant
    ON cost_limits(tenant_id)
    WHERE tenant_id IS NOT NULL;

CREATE TABLE epoch_control (
    epoch       TEXT PRIMARY KEY,
    state       TEXT NOT NULL CHECK (state IN ('draining', 'killed')),
    updated_at  TEXT NOT NULL
) STRICT;

CREATE INDEX epoch_control_state
    ON epoch_control(state);

CREATE TABLE shadow_decisions (
    shadow_decision_id  TEXT PRIMARY KEY,
    episode_id          TEXT NOT NULL,
    decision_id         TEXT NOT NULL,
    attempt_id          TEXT NOT NULL,
    fence               INTEGER NOT NULL CHECK (fence >= 0),
    decision_json       BLOB NOT NULL,
    decision_sha256     BLOB NOT NULL CHECK (length(decision_sha256) = 32),
    shadow_score        TEXT NOT NULL CHECK (
        shadow_score IN ('would_approve', 'would_require_approval', 'would_deny')
    ),
    score_reason        TEXT,
    tenant_id           TEXT NOT NULL,
    situation_id        TEXT NOT NULL,
    situation_version   INTEGER NOT NULL CHECK (situation_version >= 1),
    policy_epoch        TEXT NOT NULL,
    created_at          TEXT NOT NULL,
    FOREIGN KEY (episode_id) REFERENCES episodes(episode_id)
) STRICT;

CREATE INDEX shadow_decisions_episode
    ON shadow_decisions(episode_id);
CREATE INDEX shadow_decisions_decision
    ON shadow_decisions(decision_id);
CREATE INDEX shadow_decisions_situation
    ON shadow_decisions(tenant_id, situation_id, situation_version);

CREATE TABLE calibration_artifacts (
    artifact_id              TEXT PRIMARY KEY,
    domain                   TEXT NOT NULL,
    model_revision           TEXT NOT NULL,
    profile_digest           TEXT NOT NULL,
    prompt_sha256            TEXT NOT NULL,
    diagnosis_catalog_sha256 TEXT NOT NULL,
    policy_digest            TEXT NOT NULL,
    artifact_sha256          TEXT NOT NULL,
    active                   INTEGER NOT NULL DEFAULT 1 CHECK (active IN (0, 1)),
    created_at               TEXT NOT NULL,
    UNIQUE (domain, artifact_sha256)
) STRICT;

CREATE INDEX calibration_artifacts_active
    ON calibration_artifacts(domain, active);

CREATE TABLE shadow_comparisons (
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
    ON shadow_comparisons(tenant_id, situation_id, situation_version);

CREATE TABLE device_target_claims (
    target          TEXT PRIMARY KEY,
    device_id       TEXT NOT NULL,
    owner_epoch     TEXT NOT NULL,
    owner_instance  TEXT NOT NULL,
    boot_id         TEXT NOT NULL,
    claim_fence     INTEGER NOT NULL CHECK (claim_fence >= 1),
    lease_until     TEXT NOT NULL,
    status          TEXT NOT NULL CHECK (status IN ('active', 'released')),
    updated_at      TEXT NOT NULL
) STRICT;

CREATE INDEX device_target_claims_owner
    ON device_target_claims(owner_epoch, owner_instance, status, lease_until);

CREATE TABLE device_command_bindings (
    command_id      TEXT PRIMARY KEY,
    target          TEXT NOT NULL,
    device_id       TEXT NOT NULL,
    boot_id         TEXT NOT NULL,
    owner_epoch     TEXT NOT NULL,
    owner_instance  TEXT NOT NULL,
    command_sha256  BLOB,
    bound_at        TEXT NOT NULL
) STRICT;

CREATE INDEX device_command_bindings_device_boot
    ON device_command_bindings(device_id, boot_id, target);

CREATE TABLE device_authority_events (
    event_id        INTEGER PRIMARY KEY AUTOINCREMENT,
    target          TEXT NOT NULL,
    device_id       TEXT NOT NULL,
    event_type      TEXT NOT NULL CHECK (event_type IN (
        'claim_acquired', 'claim_renewed', 'claim_released',
        'claim_rejected', 'reconciliation_opened', 'reconciliation_recorded',
        'safe_stop_requested', 'safe_stop_completed', 'safe_stop_failed'
    )),
    owner_epoch     TEXT NOT NULL,
    owner_instance  TEXT NOT NULL,
    boot_id         TEXT NOT NULL,
    details_json    BLOB NOT NULL,
    details_sha256  BLOB NOT NULL CHECK (length(details_sha256) = 32),
    occurred_at     TEXT NOT NULL
) STRICT;

CREATE INDEX device_authority_events_target_time
    ON device_authority_events(target, occurred_at, event_id);

CREATE TABLE device_reconciliation (
    device_id                 TEXT PRIMARY KEY,
    boot_id                   TEXT NOT NULL,
    status                    TEXT NOT NULL CHECK (status IN ('clear', 'required')),
    opening_boot_id           TEXT NOT NULL,
    state_json                BLOB NOT NULL,
    state_sha256              BLOB NOT NULL CHECK (length(state_sha256) = 32),
    last_resolution_status    TEXT CHECK (last_resolution_status IS NULL OR last_resolution_status IN ('succeeded', 'failed', 'manual_review')),
    resolution_evidence_json  BLOB,
    resolution_sha256         BLOB CHECK (resolution_sha256 IS NULL OR length(resolution_sha256) = 32),
    authority_epoch           TEXT NOT NULL,
    opened_at                 TEXT NOT NULL,
    resolved_at               TEXT,
    updated_at                TEXT NOT NULL
) STRICT;

CREATE INDEX device_reconciliation_status
    ON device_reconciliation(status, updated_at);

CREATE TABLE device_safety_events (
    event_id        INTEGER PRIMARY KEY AUTOINCREMENT,
    event_type      TEXT NOT NULL CHECK (event_type IN (
        'unsafe_output', 'stale_energizing_effect',
        'duplicate_net_energizing_effect', 'unexplained_actuator_transition',
        'false_verified_success', 'safe_state_deadline_miss',
        'physical_transition'
    )),
    target          TEXT NOT NULL,
    command_id      TEXT,
    details_json    BLOB NOT NULL,
    details_sha256  BLOB NOT NULL CHECK (length(details_sha256) = 32),
    occurred_at     TEXT NOT NULL
) STRICT;

CREATE INDEX device_safety_events_type_time
    ON device_safety_events(event_type, occurred_at, event_id);

CREATE INDEX principals_tenant_status
    ON principals(tenant_id, status, principal_id);
