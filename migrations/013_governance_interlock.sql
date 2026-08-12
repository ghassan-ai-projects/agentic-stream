-- Durable governance state for approval separation and action interlocks.
CREATE TABLE principals (
    principal_id TEXT PRIMARY KEY,
    tenant_id    TEXT NOT NULL,
    status       TEXT NOT NULL CHECK (status IN ('active', 'disabled')),
    created_at   TEXT NOT NULL
) STRICT;

CREATE INDEX principals_tenant_status ON principals(tenant_id, status, principal_id);

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

INSERT INTO runtime_interlock (singleton_id, status, reason, version, updated_at)
VALUES (1, 'ready', 'initial state', 1, strftime('%Y-%m-%dT%H:%M:%fZ', 'now'));

ALTER TABLE approvals ADD COLUMN relay_identity TEXT;
ALTER TABLE approvals ADD COLUMN nonce TEXT;
ALTER TABLE approvals ADD COLUMN assertion_sha256 BLOB;
ALTER TABLE approvals ADD COLUMN withdrawn_at TEXT;
ALTER TABLE approvals ADD COLUMN withdrawal_reason TEXT;
ALTER TABLE principals ADD COLUMN public_key BLOB;
