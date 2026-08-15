-- P8: shadow decisions. A shadow dispatch persists the produced decision and
-- its would-be policy score (what EvaluateIntent WOULD have decided) but never
-- writes to intents or commands — nothing from a shadow run enters action
-- governance.
CREATE TABLE IF NOT EXISTS shadow_decisions (
    shadow_decision_id  TEXT PRIMARY KEY,
    episode_id          TEXT NOT NULL,
    decision_id         TEXT NOT NULL,
    attempt_id          TEXT NOT NULL,
    fence               INTEGER NOT NULL CHECK (fence >= 0),
    decision_json       BLOB NOT NULL,
    decision_sha256     BLOB NOT NULL CHECK (length(decision_sha256) = 32),
    shadow_score        TEXT NOT NULL CHECK (shadow_score IN (
        'would_approve',
        'would_require_approval',
        'would_deny'
    )),
    score_reason        TEXT,
    tenant_id           TEXT NOT NULL,
    situation_id        TEXT NOT NULL,
    situation_version   INTEGER NOT NULL CHECK (situation_version >= 1),
    policy_epoch        TEXT NOT NULL,
    created_at          TEXT NOT NULL,
    FOREIGN KEY (episode_id) REFERENCES episodes(episode_id)
) STRICT;

CREATE INDEX shadow_decisions_episode ON shadow_decisions (episode_id);
CREATE INDEX shadow_decisions_decision ON shadow_decisions (decision_id);
CREATE INDEX shadow_decisions_situation ON shadow_decisions (tenant_id, situation_id, situation_version);
