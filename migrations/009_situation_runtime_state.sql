-- Existing Situation rows are intentionally incompatible with the runtime
-- state codec. They remain marked for rebuild; the runtime refuses to start
-- against them rather than guessing reducer-private state.
ALTER TABLE situations ADD COLUMN state_codec_version INTEGER NOT NULL DEFAULT 0 CHECK (state_codec_version >= 0);
ALTER TABLE situations ADD COLUMN state_json BLOB NOT NULL DEFAULT X'7B7D';
ALTER TABLE situations ADD COLUMN state_sha256 BLOB NOT NULL DEFAULT X'0000000000000000000000000000000000000000000000000000000000000000' CHECK (length(state_sha256) = 32);
