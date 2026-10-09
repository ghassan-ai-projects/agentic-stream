-- Drop tables nothing reads or writes:
--   * calibration_artifacts: its only writer was deleted with qualification and
--     policy no longer reads it; consequential (R2) intents always need human
--     approval;
--   * replay_jobs: replay runs offline in an isolated database, never as a job
--     in the runtime database;
--   * episode_events: episode events are streamed and summarized in attempts
--     and decisions; nothing ever persisted or read them here.
-- artifacts stays: other tables reference it by foreign key.
DROP TABLE IF EXISTS calibration_artifacts;
DROP TABLE IF EXISTS replay_jobs;
DROP TABLE IF EXISTS episode_events;
