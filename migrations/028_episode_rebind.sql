-- ISSUE-061: re-bind bound episodes to the live situation version at dispatch.
-- NEW column on the existing episodes table (no existing constraint changes),
-- the 024 pattern: plain ADD COLUMN, no rebuild, no FK churn.
ALTER TABLE episodes ADD COLUMN stale_rebind_count INTEGER NOT NULL DEFAULT 0;
