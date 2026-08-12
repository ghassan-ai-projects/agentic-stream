-- Durable policy decisions make the authorization boundary explainable and
-- allow a later approval or reconciliation step to refer to the exact check.

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
