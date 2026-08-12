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
    updated_at           TEXT NOT NULL
) STRICT;

CREATE INDEX watch_conditions_due
    ON watch_conditions(tenant_id, status, expires_at);

CREATE TABLE watch_fires (
    watch_id             TEXT NOT NULL REFERENCES watch_conditions(watch_id),
    event_id             TEXT NOT NULL,
    fired_at             TEXT NOT NULL,
    PRIMARY KEY (watch_id, event_id)
) STRICT;
