-- Phase 04: durable target authority, device reboot reconciliation, and
-- safety evidence. These records are intentionally separate from the action
-- outbox: they explain authority and safety state without becoming a second
-- command-delivery path.

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
    authority_epoch            TEXT NOT NULL,
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
