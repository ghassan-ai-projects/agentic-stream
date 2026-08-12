-- Runtime ownership is a singleton lease. A live process must hold it before
-- it may expose readiness or dispatch work.

CREATE TABLE runtime_owner (
    singleton_id INTEGER PRIMARY KEY CHECK (singleton_id = 1),
    owner_epoch  TEXT NOT NULL,
    owner_instance TEXT NOT NULL,
    acquired_at  TEXT NOT NULL,
    heartbeat_at TEXT NOT NULL,
    lease_until  TEXT NOT NULL
) STRICT;
