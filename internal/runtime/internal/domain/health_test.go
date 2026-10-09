package domain

import "testing"

func TestIngestLagIsTheLogHeadAheadOfTheEngine(t *testing.T) {
	t.Parallel()
	if lag := (HealthCounts{LogHead: 10, AppliedPosition: 7}).Gauges()["agentic_stream_ingest_lag_events"]; lag != 3 {
		t.Fatalf("lag = %v, want 3", lag)
	}
	if lag := (HealthCounts{LogHead: 5, AppliedPosition: 9}).Gauges()["agentic_stream_ingest_lag_events"]; lag != 0 {
		t.Fatalf("lag = %v, want 0 when the engine reports a later position", lag)
	}
}
