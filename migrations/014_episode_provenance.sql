ALTER TABLE episodes ADD COLUMN prompt_sha256 BLOB CHECK (prompt_sha256 IS NULL OR length(prompt_sha256) = 32);
ALTER TABLE episodes ADD COLUMN objective_sha256 BLOB CHECK (objective_sha256 IS NULL OR length(objective_sha256) = 32);
