-- P4 (PHASE_P4_INTENT_AUTHORITY): the catalog's per-intent policy and hourly
-- rate limit, enforced at dispatch by the policy gateway (EvaluateIntent).
-- The declared policy/limit are the digest-bound catalog's; this migration
-- adds the storage for them and the atomic dispatch counters the gate
-- increments.
ALTER TABLE intents ADD COLUMN rate_limit_per_hour INTEGER NOT NULL DEFAULT 0;
ALTER TABLE intents ADD COLUMN requires_approval INTEGER NOT NULL DEFAULT 0;

CREATE TABLE IF NOT EXISTS intent_dispatch_counts (
    tenant_id   TEXT NOT NULL,
    intent_type TEXT NOT NULL,
    bucket      TEXT NOT NULL,
    count       INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (tenant_id, intent_type, bucket)
);
