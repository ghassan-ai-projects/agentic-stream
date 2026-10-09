-- An event the engine could not apply because applying it fails the same way
-- every time (an operator or Situation rule refused it). The failure is kept
-- here and the event is marked processed, so one bad record cannot halt every
-- later event of the tenant.
CREATE TABLE apply_failures (
    tenant_id           TEXT NOT NULL,
    event_id            TEXT NOT NULL,
    log_position        INTEGER NOT NULL REFERENCES event_log(position),
    partition_id        INTEGER NOT NULL CHECK (partition_id >= 0),
    step                TEXT NOT NULL,
    error_text          TEXT NOT NULL,
    recorded_at         TEXT NOT NULL,
    PRIMARY KEY (tenant_id, event_id)
) STRICT;
