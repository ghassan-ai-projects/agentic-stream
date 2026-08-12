-- Active attempts are fenced to the runtime epoch that dispatched them.
-- NULL is retained only for pre-epoch terminal/history rows; startup recovery
-- treats a NULL epoch on an active attempt as orphaned.

ALTER TABLE episode_attempts ADD COLUMN owner_epoch TEXT;
