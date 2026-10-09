-- Indexes for the queries the runtime issues on every event or batch.
--   * event_log_tenant_position: the log head and the global page read filter
--     on the tenant and order by position across partitions.
--   * timers_pending_due: due processing-time timers per partition; partial,
--     so fired and cancelled timers never slow the lookup.
--   * timers_pending_state: cancelling the pending heartbeat timer of one
--     operator state key.
--   * intents_pending: the oldest intent still awaiting policy evaluation.
--   * watch_conditions_active_expiry: expiring active watches across tenants.
CREATE INDEX event_log_tenant_position
    ON event_log(tenant_id, position);

CREATE INDEX timers_pending_due
    ON timers(deployment_id, tenant_id, partition_id, due_at)
    WHERE status = 'pending' AND timer_kind = 'processing_time';

CREATE INDEX timers_pending_state
    ON timers(deployment_id, tenant_id, partition_id, operator_id, state_key, timer_kind)
    WHERE status = 'pending';

CREATE INDEX intents_pending
    ON intents(tenant_id, created_at, intent_id)
    WHERE policy_status = 'pending';

CREATE INDEX watch_conditions_active_expiry
    ON watch_conditions(status, expires_at);
