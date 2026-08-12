-- Durable bounded retry state for malformed or tampered notification rows.

CREATE TABLE notification_poison_attempts (
    tenant_id       TEXT NOT NULL,
    cursor          INTEGER NOT NULL CHECK (cursor >= 1),
    attempts        INTEGER NOT NULL CHECK (attempts >= 1),
    last_attempt_at TEXT NOT NULL,
    PRIMARY KEY (tenant_id, cursor),
    FOREIGN KEY (tenant_id, cursor) REFERENCES notifications(tenant_id, cursor)
) STRICT;
