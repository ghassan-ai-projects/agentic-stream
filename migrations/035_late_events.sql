-- Durable decision for every event that arrived behind its partition's
-- watermark: whether the late-data policy let it correct state, kept it as
-- history only, dropped it, or refused it as later than the allowed lateness.
CREATE TABLE late_events (
    tenant_id           TEXT NOT NULL,
    event_id            TEXT NOT NULL,
    log_position        INTEGER NOT NULL REFERENCES event_log(position),
    partition_id        INTEGER NOT NULL CHECK (partition_id >= 0),
    event_time          TEXT NOT NULL,
    watermark           TEXT NOT NULL,
    late_policy         TEXT NOT NULL,
    disposition         TEXT NOT NULL CHECK (
        disposition IN ('corrected', 'history_only', 'dropped', 'beyond_allowed_lateness')
    ),
    recorded_at         TEXT NOT NULL,
    PRIMARY KEY (tenant_id, event_id)
) STRICT;
