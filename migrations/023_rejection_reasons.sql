-- P4: the rejection registry gains the intent-catalog reasons. SQLite cannot
-- ALTER a CHECK constraint, so the table is rebuilt with the widened list.
DROP TABLE IF EXISTS episode_rejections;

CREATE TABLE episode_rejections (
    rejection_id  TEXT PRIMARY KEY,
    episode_id    TEXT,
    attempt_id    TEXT,
    fence         INTEGER NOT NULL CHECK (fence >= 0),
    reason        TEXT NOT NULL CHECK (
        reason IN (
            'unknown_episode',
            'stale_attempt',
            'wrong_attempt',
            'terminal_attempt',
            'episode_closed',
            'schema_invalid',
            'snapshot_mismatch',
            'evidence_not_visible',
            'forged_reference',
            'oversized',
            'expired',
            'intent_type_not_allowed',
            'risk_ceiling_exceeded',
            'catalog_missing',
            'catalog_forged',
            'intent_type_not_in_catalog',
            'risk_label_mismatch',
            'parameter_schema_violation',
            'preset_mismatch',
            'ungrounded_evidence'
        )
    ),
    details_json  BLOB NOT NULL,
    created_at    TEXT NOT NULL,
    FOREIGN KEY (episode_id) REFERENCES episodes(episode_id)
) STRICT;

CREATE INDEX episode_rejections_identity
    ON episode_rejections (episode_id, attempt_id, fence);
