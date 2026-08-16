-- P8: the epoch control record. One row per epoch that has been drained or
-- killed. `killed` refuses EVERY later decision under that epoch (in-flight
-- included, independently of the worker); `draining` refuses only new
-- admission — in-flight episodes finish under their RECORDED policy_epoch.
CREATE TABLE IF NOT EXISTS epoch_control (
    epoch       TEXT PRIMARY KEY,
    state       TEXT NOT NULL CHECK (state IN ('draining', 'killed')),
    updated_at  TEXT NOT NULL
) STRICT;

CREATE INDEX epoch_control_state ON epoch_control (state);
