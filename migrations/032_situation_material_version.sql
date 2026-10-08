-- ADR-018: cognition records the latest material version of each Situation in
-- the transaction that publishes it. Policy, approval resolution and dispatch
-- refuse an intent only when a material version newer than the intent's exists.
-- Existing rows start strict: every version up to the current one is material.
ALTER TABLE situations ADD COLUMN last_material_version INTEGER NOT NULL DEFAULT 0 CHECK (last_material_version >= 0);
UPDATE situations SET last_material_version = current_version;
