-- Durable Channel B notification log. Cursor is monotonic per tenant.

CREATE TABLE notifications (
    tenant_id       TEXT NOT NULL,
    cursor          INTEGER NOT NULL CHECK (cursor >= 1),
    event_id        TEXT NOT NULL,
    event_type      TEXT NOT NULL,
    event_json      BLOB NOT NULL,
    event_sha256    BLOB NOT NULL CHECK (length(event_sha256) = 32),
    created_at      TEXT NOT NULL,
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

CREATE INDEX notifications_retention
    ON notifications(tenant_id, created_at, cursor);

CREATE TABLE notification_audits (
    audit_id        TEXT PRIMARY KEY,
    tenant_id       TEXT NOT NULL,
    action          TEXT NOT NULL CHECK (action IN ('cursor_expired', 'subscriber_skipped', 'subscriber_too_slow')),
    requested_cursor INTEGER,
    oldest_cursor   INTEGER,
    details_json    BLOB NOT NULL,
    created_at      TEXT NOT NULL
) STRICT;
