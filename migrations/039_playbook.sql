-- Rule promotion (AS-3): the playbook's durable evidence. Rules themselves
-- live in the playbook file; these tables record what the rules did. They have
-- no foreign key into intents, commands or the outbox and authorize nothing.

-- One row per rule episode admitted: the rate cap counts them by the event
-- time of the Situation version they answered, the audit sample is drawn when
-- the fire is recorded.
CREATE TABLE rule_fires (
    fire_id            TEXT PRIMARY KEY,
    tenant_id          TEXT NOT NULL,
    rule_id            TEXT NOT NULL,
    rule_version       INTEGER NOT NULL CHECK (rule_version >= 1),
    playbook_digest    TEXT NOT NULL,
    episode_id         TEXT NOT NULL UNIQUE,
    situation_id       TEXT NOT NULL,
    situation_version  INTEGER NOT NULL CHECK (situation_version >= 1),
    recommendation_json BLOB NOT NULL,
    audit_due          INTEGER NOT NULL CHECK (audit_due IN (0, 1)),
    audit_queued       INTEGER NOT NULL DEFAULT 0 CHECK (audit_queued IN (0, 1)),
    event_time         TEXT NOT NULL
) STRICT;

CREATE INDEX rule_fires_rule_time ON rule_fires (rule_id, rule_version, event_time);
CREATE INDEX rule_fires_audit ON rule_fires (audit_due, audit_queued);

-- What a rule would have recommended next to the model's own decision: a
-- shadow rule beside a model episode, or an audit episode beside a rule fire.
CREATE TABLE rule_evaluations (
    evaluation_id       TEXT PRIMARY KEY,
    tenant_id           TEXT NOT NULL,
    rule_id             TEXT NOT NULL,
    rule_version        INTEGER NOT NULL CHECK (rule_version >= 1),
    playbook_digest     TEXT NOT NULL,
    kind                TEXT NOT NULL CHECK (kind IN ('shadow', 'audit')),
    fire_id             TEXT REFERENCES rule_fires(fire_id),
    scheduler_item_id   TEXT UNIQUE,
    episode_id          TEXT,
    situation_id        TEXT NOT NULL,
    situation_version   INTEGER NOT NULL CHECK (situation_version >= 1),
    in_envelope         INTEGER NOT NULL CHECK (in_envelope IN (0, 1)),
    recommendation_json BLOB NOT NULL,
    verdict             TEXT NOT NULL CHECK (verdict IN ('pending', 'agree', 'disagree', 'inconclusive')),
    observed_json       BLOB,
    event_time          TEXT NOT NULL,
    created_at          TEXT NOT NULL,
    settled_at          TEXT
) STRICT;

CREATE INDEX rule_evaluations_pending ON rule_evaluations (verdict, kind);
CREATE INDEX rule_evaluations_rule ON rule_evaluations (rule_id, rule_version, kind, verdict);
CREATE INDEX rule_evaluations_rule_time ON rule_evaluations (rule_id, rule_version, kind, event_time);

-- An open circuit switches its rule off until an operator closes it
-- (agentic-stream circuits close, with the runtime stopped).
CREATE TABLE rule_circuits (
    rule_id        TEXT NOT NULL,
    rule_version   INTEGER NOT NULL CHECK (rule_version >= 1),
    tenant_id      TEXT NOT NULL,
    state          TEXT NOT NULL CHECK (state IN ('open', 'closed')),
    cause          TEXT NOT NULL CHECK (cause IN ('audit_disagreement', 'rate_cap', 'kind_changed', 'no_effect', 'out_of_cap')),
    evidence_json  BLOB NOT NULL,
    opened_at      TEXT NOT NULL,
    closed_at      TEXT,
    close_reason   TEXT,
    PRIMARY KEY (rule_id, rule_version)
) STRICT;

-- The digest each device kind was last born with, as the playbook last saw it
-- from plant wire: routing, case keys and drafts read it, so it survives a
-- restart until the next birth.
CREATE TABLE rule_kind_digests (
    tenant_id    TEXT NOT NULL,
    kind         TEXT NOT NULL,
    digest       TEXT NOT NULL,
    recorded_at  TEXT NOT NULL,
    PRIMARY KEY (tenant_id, kind)
) STRICT;
