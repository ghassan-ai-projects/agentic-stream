-- Rule promotion (AS-7): draft calls to the worker and the rule proposals a
-- human approves. A proposal carries the runtime-built rule (condition from
-- the worker; envelope, cap, mode and caps from the runtime) and the replay
-- report the approver reads. An approved proposal is written to the playbook
-- file, in shadow; nothing here authorizes an action.
CREATE TABLE rule_drafts (
    draft_id         TEXT PRIMARY KEY,
    tenant_id        TEXT NOT NULL,
    diagnosis_code   TEXT NOT NULL,
    kind_digest      TEXT NOT NULL,
    intent           TEXT NOT NULL,
    preset           TEXT NOT NULL,
    status           TEXT NOT NULL CHECK (status IN ('requested', 'proposed', 'no_proposal', 'refused', 'failed')),
    attempts         INTEGER NOT NULL DEFAULT 0 CHECK (attempts >= 0),
    evidence_cases   INTEGER NOT NULL CHECK (evidence_cases >= 0),
    request_json     BLOB,
    reply_json       BLOB,
    reason           TEXT NOT NULL DEFAULT '',
    input_tokens     INTEGER NOT NULL DEFAULT 0,
    output_tokens    INTEGER NOT NULL DEFAULT 0,
    cost_microunits  INTEGER NOT NULL DEFAULT 0,
    created_at       TEXT NOT NULL,
    updated_at       TEXT NOT NULL
) STRICT;

CREATE INDEX rule_drafts_key ON rule_drafts (tenant_id, diagnosis_code, kind_digest, intent, preset, status);

CREATE TABLE rule_proposals (
    proposal_id       TEXT PRIMARY KEY,
    tenant_id         TEXT NOT NULL,
    draft_id          TEXT NOT NULL UNIQUE REFERENCES rule_drafts(draft_id),
    diagnosis_code    TEXT NOT NULL,
    kind_digest       TEXT NOT NULL,
    intent            TEXT NOT NULL,
    preset            TEXT NOT NULL,
    risk_class        TEXT NOT NULL,
    rule_json         BLOB NOT NULL,
    proposal_digest   TEXT NOT NULL,
    replay_json       BLOB NOT NULL,
    evidence_cases    INTEGER NOT NULL CHECK (evidence_cases >= 0),
    status            TEXT NOT NULL CHECK (status IN ('pending', 'approved', 'rejected', 'written', 'refused')),
    approver_id       TEXT NOT NULL DEFAULT '',
    relay_id          TEXT NOT NULL DEFAULT '',
    approval_digest   TEXT NOT NULL DEFAULT '',
    reason            TEXT NOT NULL DEFAULT '',
    written_digest    TEXT NOT NULL DEFAULT '',
    created_at        TEXT NOT NULL,
    updated_at        TEXT NOT NULL
) STRICT;

CREATE INDEX rule_proposals_key ON rule_proposals (tenant_id, diagnosis_code, kind_digest, intent, preset, status);
