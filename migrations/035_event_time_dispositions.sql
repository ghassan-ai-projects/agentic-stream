-- Durable decision for every event the engine did not apply as an ordinary
-- on-time event: a late event the late-data policy let correct state, kept as
-- history only, dropped, or refused as later than the allowed lateness; or an
-- event whose event time ran ahead of its ingestion beyond the clock-skew
-- tolerance.
CREATE TABLE event_time_dispositions (
    tenant_id           TEXT NOT NULL,
    event_id            TEXT NOT NULL,
    log_position        INTEGER NOT NULL REFERENCES event_log(position),
    partition_id        INTEGER NOT NULL CHECK (partition_id >= 0),
    event_time          TEXT NOT NULL,
    watermark           TEXT NOT NULL,
    late_policy         TEXT NOT NULL,
    disposition         TEXT NOT NULL CHECK (
        disposition IN ('corrected', 'history_only', 'dropped', 'beyond_allowed_lateness', 'clock_skew')
    ),
    recorded_at         TEXT NOT NULL,
    PRIMARY KEY (tenant_id, event_id)
) STRICT;

ALTER TABLE partition_checkpoints ADD COLUMN sources_json BLOB;
