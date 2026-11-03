-- One rule-handled case: pump p1 opened (v1, 06:00), the bearing-wear rule
-- booked maintenance (acknowledged, manual review), the situation resolved
-- (v2, 08:00, its vibration back near the 3.0 baseline of the decision), and the event log reached the next day.
INSERT INTO situations (situation_id, tenant_id, deployment_id, situation_type, entity_type, entity_id, partition_id, occurrence_id,
    current_version, phase, status, first_event_time, latest_event_time, updated_at, created_at)
VALUES ('s1', 't', 'dep', 'pump_condition', 'pump', 'p1', 0, 'occ-1', 2, 'resolved', 'active', 'x', 'x', 'x', 'x');
INSERT INTO situation_versions (situation_id, version, phase, severity, confidence, completeness, event_horizon, valid_from,
    snapshot_json, snapshot_sha256, lineage_id, created_at)
VALUES ('s1', 1, 'watch', 40, 1, 'on_time', '2026-01-01T06:00:00.000000000Z', 'x',
    CAST('{"facts":{"facts.vibration_mean_15m":4.2,"facts.vibration_mean_24h":3.0,"facts.current_mean_15m":30.0},"evidence":[]}' AS BLOB),
    zeroblob(32), 'l', 'x'),
    ('s1', 2, 'resolved', 0, 1, 'on_time', '2026-01-01T08:00:00.000000000Z', 'x', CAST('{"facts":{"facts.vibration_mean_15m":3.1,"facts.vibration_mean_24h":3.0,"facts.current_mean_15m":30.0},"evidence":[]}' AS BLOB), zeroblob(32), 'l', 'x');
INSERT INTO episodes (episode_id, scheduler_item_id, tenant_id, situation_id, situation_version, executor_name, executor_version,
    model_policy, prompt_version, snapshot_sha256, admission_key, request_json, lifecycle_status, accepted_at, dispatch_policy)
VALUES ('e1', 'item-1', 't', 's1', 1, 'rule', 'sha256:pb', 'p', 'v', zeroblob(32), randomblob(32), CAST('{}' AS BLOB), 'concluded', 'x', 'active');
INSERT INTO decisions (decision_id, episode_id, ordinal, situation_id, situation_version, raw_json, decision_sha256,
    validation_status, validation_json, created_at)
VALUES ('d1', 'e1', 1, 's1', 1, CAST('{"diagnosis_code":"bearing_wear","summary":"rule bearing-wear v1","intents":[{"type":"schedule_maintenance","parameters":{"work_order":"bearing_wear"}}]}' AS BLOB),
    zeroblob(32), 'accepted', CAST('{}' AS BLOB), 'x');
INSERT INTO intents (intent_id, decision_id, tenant_id, situation_id, situation_version, intent_type, risk_class, intent_json,
    intent_sha256, expires_at, policy_status, created_at, updated_at)
VALUES ('i1', 'd1', 't', 's1', 1, 'schedule_maintenance', 'R1', CAST('{}' AS BLOB), zeroblob(32), 'x', 'approved', 'x', 'x');
INSERT INTO commands (command_id, intent_id, tenant_id, effector_route, normalized_target, idempotency_key, command_json,
    command_sha256, status, created_at, updated_at)
VALUES ('c1', 'i1', 't', 'schedule_maintenance', 'p1', randomblob(32), CAST('{}' AS BLOB), zeroblob(32), 'manual_review', 'x', 'x');
INSERT INTO rule_fires (fire_id, tenant_id, rule_id, rule_version, playbook_digest, episode_id, situation_id, situation_version,
    recommendation_json, audit_due, event_time)
VALUES ('rfire_e1', 't', 'bearing-wear', 1, 'sha256:pb', 'e1', 's1', 1, CAST('{}' AS BLOB), 0, '2026-01-01T06:00:00.000000000Z');
INSERT INTO event_log (tenant_id, partition_id, event_id, event_type, schema_version, source, partition_key, entity_type, entity_id,
    event_time, ingested_at, classification, quality_json, payload_json, payload_sha256, created_at)
VALUES ('t', 0, 'ev-1', 'pump.vibration.observed', '1.1', 'test', 'p1', 'pump', 'p1', '2026-01-02T08:00:00.000000000Z', 'x',
    'internal', CAST('{}' AS BLOB), CAST('{}' AS BLOB), zeroblob(32), 'x');
