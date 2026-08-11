-- Split episode coordination, worker attempts, decision validation, and
-- downstream verification into independent state machines.
--
-- This is a breaking schema cutover. The old episodes.status values were a
-- conflation of execution and business outcomes and are intentionally not
-- retained as a compatibility column.

PRAGMA foreign_keys = ON;
-- The child tables are temporarily removed while their parent is rebuilt.
-- Deferral lets existing intent/command rows keep their decision references;
-- the replacement decision rows are restored before this migration commits.
PRAGMA defer_foreign_keys = ON;

-- The two child tables are rebuilt after the parent so their foreign keys point
-- at the new episodes table rather than the temporary legacy table.
CREATE TEMP TABLE episode_events_before_lifecycle AS
SELECT episode_id, sequence, event_type, event_json, event_sha256, durable, occurred_at
FROM episode_events;

CREATE TEMP TABLE decisions_before_lifecycle AS
SELECT decision_id, episode_id, ordinal, situation_id, situation_version,
       raw_json, decision_sha256, validation_status, validation_json, created_at
FROM decisions;

DROP TABLE episode_events;
DROP TABLE decisions;

DROP INDEX episodes_situation_status;
DROP INDEX one_live_episode_per_situation;

ALTER TABLE episodes RENAME TO episodes_before_lifecycle;

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
    FOREIGN KEY (situation_id, situation_version)
        REFERENCES situation_versions(situation_id, version)
) STRICT;

-- Migration map from the conflated status to aggregate coordination only:
-- accepted/queued -> admitted;
-- running/cancelling -> abandoned because no attempt identity existed to
-- safely resume the work;
-- decided/no_action/needs_human/timed_out/budget_exhausted/failed/cancelled/
-- interrupted -> concluded;
-- superseded -> superseded.
INSERT INTO episodes (
    episode_id, scheduler_item_id, tenant_id, situation_id, situation_version,
    executor_name, executor_version, model_policy, prompt_version,
    snapshot_sha256, admission_key, request_json, lifecycle_status,
    current_attempt_id, current_fence, accepted_at, started_at, ended_at,
    terminal_json
)
SELECT
    episode_id, scheduler_item_id, tenant_id, situation_id, situation_version,
    executor_name, executor_version, model_policy, prompt_version,
    snapshot_sha256, admission_key, request_json,
    CASE status
        WHEN 'accepted' THEN 'admitted'
        WHEN 'queued' THEN 'admitted'
        WHEN 'running' THEN 'abandoned'
        WHEN 'cancelling' THEN 'abandoned'
        WHEN 'superseded' THEN 'superseded'
        ELSE 'concluded'
    END,
    NULL, 0, accepted_at, started_at, ended_at, terminal_json
FROM episodes_before_lifecycle;

DROP TABLE episodes_before_lifecycle;

CREATE INDEX episodes_situation_lifecycle
    ON episodes(situation_id, lifecycle_status, accepted_at);

CREATE UNIQUE INDEX one_live_episode_per_situation
    ON episodes(situation_id)
    WHERE lifecycle_status IN ('admitted', 'running');

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
    UNIQUE (episode_id, fence)
) STRICT;

CREATE INDEX episode_attempts_live
    ON episode_attempts(episode_id, status, fence DESC);

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

INSERT INTO episode_events (
    episode_id, sequence, attempt_id, fence, event_type, event_json,
    event_sha256, durable, occurred_at
)
SELECT episode_id, sequence, NULL, NULL, event_type, event_json,
       event_sha256, durable, occurred_at
FROM episode_events_before_lifecycle;

DROP TABLE episode_events_before_lifecycle;

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
    UNIQUE (episode_id, ordinal),
    FOREIGN KEY (situation_id, situation_version)
        REFERENCES situation_versions(situation_id, version)
) STRICT;

INSERT INTO decisions (
    decision_id, episode_id, attempt_id, fence, ordinal, situation_id,
    situation_version, raw_json, decision_sha256, validation_status,
    rejection_reason, validation_json, created_at
)
SELECT decision_id, episode_id, NULL, NULL, ordinal, situation_id,
       situation_version, raw_json, decision_sha256, validation_status,
       NULL, validation_json, created_at
FROM decisions_before_lifecycle;

DROP TABLE decisions_before_lifecycle;

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
            'risk_ceiling_exceeded'
        )
    ),
    details_json  BLOB NOT NULL,
    created_at    TEXT NOT NULL,
    FOREIGN KEY (episode_id) REFERENCES episodes(episode_id)
) STRICT;

CREATE INDEX episode_rejections_identity
    ON episode_rejections(episode_id, fence, created_at);

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
