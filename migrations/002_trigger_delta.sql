-- Add persisted delta to trigger evaluations so the episode assembler can build
-- a deterministic snapshot without re-loading the previous reasoned version.

ALTER TABLE trigger_evaluations ADD COLUMN delta_json BLOB NOT NULL DEFAULT X'7B7D';
