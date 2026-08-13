-- Preserve source trace context through accepted decisions, outcomes, and
-- durable Channel-B notifications without changing Decision v1 JSON.

ALTER TABLE decisions ADD COLUMN traceparent TEXT;
ALTER TABLE decisions ADD COLUMN tracestate TEXT;
ALTER TABLE outcomes ADD COLUMN traceparent TEXT;
ALTER TABLE outcomes ADD COLUMN tracestate TEXT;
ALTER TABLE notifications ADD COLUMN traceparent TEXT;
ALTER TABLE notifications ADD COLUMN tracestate TEXT;
