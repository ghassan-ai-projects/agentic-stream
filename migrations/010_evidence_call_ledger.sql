-- Durable evidence-call reservation and result ledger. Capability secrets are
-- deliberately absent; only non-secret token metadata is retained.
CREATE UNIQUE INDEX episode_attempt_identity
    ON episode_attempts(attempt_id, episode_id, fence);

CREATE TABLE evidence_call_ledger (
    tenant_id             TEXT NOT NULL,
    episode_id            TEXT NOT NULL REFERENCES episodes(episode_id),
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
    FOREIGN KEY (attempt_id, episode_id, fence) REFERENCES episode_attempts(attempt_id, episode_id, fence),
    CHECK (status != 'completed' OR (result_json IS NOT NULL AND result_sha256 IS NOT NULL AND result_bytes IS NOT NULL AND row_count IS NOT NULL AND completed_at IS NOT NULL)),
    CHECK (status = 'running' OR result_json IS NOT NULL OR result_sha256 IS NULL)
) STRICT;

CREATE INDEX evidence_call_ledger_leases
    ON evidence_call_ledger(status, lease_until);

CREATE INDEX evidence_call_ledger_attempt
    ON evidence_call_ledger(tenant_id, episode_id, attempt_id, fence);
