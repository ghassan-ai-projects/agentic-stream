package domain

import (
	"testing"
	"time"
)

func TestTheEvidenceHorizonIsTheBoundSnapshotsEventHorizon(t *testing.T) {
	t.Parallel()
	request := Request{EpisodeID: "e1", RequestJSON: []byte(`{"snapshot":{"event_horizon":"2026-01-01T10:00:00.000000000Z"}}`)}
	horizon, err := request.EvidenceHorizon()
	if err != nil || !horizon.Equal(time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)) {
		t.Fatalf("horizon = %s err=%v", horizon, err)
	}
	for _, raw := range []string{`{}`, `not json`, `{"snapshot":{"event_horizon":"soon"}}`} {
		if _, err := (&Request{RequestJSON: []byte(raw)}).EvidenceHorizon(); err == nil {
			t.Fatalf("request %s gave a horizon", raw)
		}
	}
}
