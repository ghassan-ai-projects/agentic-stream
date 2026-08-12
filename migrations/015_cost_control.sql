CREATE TABLE cost_limits (
    scope_key       TEXT PRIMARY KEY,
    tenant_id       TEXT,
    max_micro       INTEGER NOT NULL CHECK (max_micro >= 0),
    reserved_micro  INTEGER NOT NULL DEFAULT 0 CHECK (reserved_micro >= 0),
    spent_micro     INTEGER NOT NULL DEFAULT 0 CHECK (spent_micro >= 0),
    kill_switch     INTEGER NOT NULL DEFAULT 0 CHECK (kill_switch IN (0, 1)),
    updated_at      TEXT NOT NULL
) STRICT;

CREATE UNIQUE INDEX cost_limits_tenant ON cost_limits(tenant_id) WHERE tenant_id IS NOT NULL;

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

INSERT INTO cost_limits (scope_key, max_micro, updated_at)
VALUES ('global', 0, strftime('%Y-%m-%dT%H:%M:%fZ', 'now'));
