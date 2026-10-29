-- The latest event time of a tenant's log (rule case verification reads it on
-- every settle) without scanning the log.
CREATE INDEX event_log_tenant_time ON event_log(tenant_id, event_time);
