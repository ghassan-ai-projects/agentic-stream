-- Rule promotion (AS-5): one record per case, an occurrence of a Situation
-- (until it resolves) whose decision dispatched an action. A case is verified only when its action was
-- acknowledged, the Situation closed and no new Situation opened on the same
-- entity before the settle window of the case's diagnosis code ended. These
-- records are the only evidence rule proposals count.
CREATE TABLE case_verifications (
    situation_id       TEXT NOT NULL,
    tenant_id          TEXT NOT NULL,
    entity_id          TEXT NOT NULL,
    situation_type     TEXT NOT NULL,
    episode_id         TEXT NOT NULL,
    decision_id        TEXT NOT NULL,
    situation_version  INTEGER NOT NULL CHECK (situation_version >= 1),
    diagnosis_code     TEXT NOT NULL,
    kind_digest        TEXT NOT NULL,
    intent             TEXT NOT NULL,
    preset             TEXT NOT NULL,
    rule_id            TEXT NOT NULL,
    rule_version       INTEGER NOT NULL CHECK (rule_version >= 0),
    acted_at           TEXT NOT NULL,
    window_ends_at     TEXT NOT NULL,
    features_json      BLOB NOT NULL,
    summary            TEXT NOT NULL,
    verdict            TEXT NOT NULL CHECK (verdict IN ('pending', 'verified', 'not_verified')),
    reason             TEXT NOT NULL,
    created_at         TEXT NOT NULL,
    settled_at         TEXT,
    PRIMARY KEY (situation_id, situation_version)
) STRICT;

CREATE INDEX case_verifications_key ON case_verifications (tenant_id, diagnosis_code, kind_digest, intent, preset, verdict);
CREATE INDEX case_verifications_pending ON case_verifications (tenant_id, verdict);
