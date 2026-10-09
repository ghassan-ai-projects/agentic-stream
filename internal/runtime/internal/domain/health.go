package domain

type HealthCounts struct {
	LogHead, AppliedPosition                    int64
	PendingSchedulerItems, OutboxOpen           int64
	VerificationsAwaiting, VerificationsRefuted int64
	ApplyFailures, DatabaseBytes                int64
}

func (c HealthCounts) Gauges() map[string]float64 {
	return map[string]float64{
		"agentic_stream_event_log_head_position": float64(c.LogHead),
		"agentic_stream_engine_applied_position": float64(c.AppliedPosition),
		"agentic_stream_ingest_lag_events":       float64(max(c.LogHead-c.AppliedPosition, 0)),
		"agentic_stream_scheduler_items_pending": float64(c.PendingSchedulerItems),
		"agentic_stream_outbox_open":             float64(c.OutboxOpen),
		"agentic_stream_verifications_awaiting":  float64(c.VerificationsAwaiting),
		"agentic_stream_verifications_refuted":   float64(c.VerificationsRefuted),
		"agentic_stream_apply_failures":          float64(c.ApplyFailures),
		"agentic_stream_database_bytes":          float64(c.DatabaseBytes),
	}
}
