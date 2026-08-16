-- P8: the mode matrix becomes durable. Every episode records the dispatch
-- policy (active | shadow) it was admitted under and the policy epoch that
-- governs it. These are NEW columns on the existing episodes table (no
-- existing constraint changes), so plain ADD COLUMN suffices — no rebuild,
-- no child-table FK churn (the 014 pattern).
ALTER TABLE episodes ADD COLUMN dispatch_policy TEXT NOT NULL DEFAULT 'shadow'
    CHECK (dispatch_policy IN ('active', 'shadow'));
ALTER TABLE episodes ADD COLUMN policy_epoch TEXT NOT NULL DEFAULT '';
