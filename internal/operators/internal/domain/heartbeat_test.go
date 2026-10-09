package domain_test

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/contractsv1"
	domain "github.com/ghassan-ai-projects/agentic-stream/internal/operators/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/spec"
)

func TestHeartbeatEventReportsPresenceAtTheWatermark(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name          string
		watermarkLag  time.Duration
		wantMissing   bool
		wantCompleted domain.Completeness
	}{
		{"fresh heartbeat", 0, false, domain.CompletenessOnTime},
		{"just inside the deadline", 5*time.Minute - time.Nanosecond, false, domain.CompletenessOnTime},
		{"exactly at the deadline", 5 * time.Minute, true, domain.CompletenessUncertain},
		{"watermark far past the deadline", 6 * time.Minute, true, domain.CompletenessUncertain},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			h := newHarness(t, heartbeatSpec())
			processing := base.Add(30 * time.Second)
			features := h.apply(heartbeat("hb", base, nil), base.Add(tc.watermarkLag), processing)
			if len(features) != 1 {
				t.Fatalf("features = %d, want 1", len(features))
			}
			got := features[0]
			if got.Value != tc.wantMissing || got.Completeness != string(tc.wantCompleted) {
				t.Fatalf("feature = value %v completeness %q, want %v %q", got.Value, got.Completeness, tc.wantMissing, tc.wantCompleted)
			}
			wantTime := base
			if tc.wantMissing {
				wantTime = processing
			}
			if !got.EventTime.Equal(wantTime) {
				t.Fatalf("event time = %s, want %s: a missing heartbeat is timed at processing time", got.EventTime, wantTime)
			}
		})
	}
}

func TestHeartbeatEventFallsBackToItsEventTimeWithoutProcessingTime(t *testing.T) {
	t.Parallel()
	h := newHarness(t, heartbeatSpec())
	h.apply(heartbeat("hb", base, nil), base, time.Time{})
	if features := h.fireTimer(base.Add(5 * time.Minute)); len(features) != 1 {
		t.Fatalf("timer features = %d, want 1: liveness is measured from the event time", len(features))
	}
}

func TestMissingHeartbeatTimerIsTimedAtDetection(t *testing.T) {
	t.Parallel()
	h := newHarness(t, heartbeatSpec())
	seen := heartbeat("hb-1", base, nil)
	seen.IngestedAt = base.Add(30 * time.Second)
	seen.Traceparent = "00-0af7651916cd43dd8448eb211c80319c-b7ad6b7169203331-01"
	first := h.apply(seen, base, seen.IngestedAt)
	if len(first) != 1 || first[0].Value != false {
		t.Fatalf("heartbeat feature = %+v, want one false feature", first)
	}
	detection := seen.IngestedAt.Add(5 * time.Minute)
	features := h.fireTimer(detection)
	if len(features) != 1 {
		t.Fatalf("timer features = %d, want 1", len(features))
	}
	got := features[0]
	if got.Value != true || got.Completeness != string(domain.CompletenessUncertain) || got.EntityType != "motor" || got.EntityID != "motor-17" {
		t.Fatalf("timer feature = %+v, want an uncertain true feature for motor-17", got)
	}
	if !got.EventTime.Equal(detection) || !got.WindowStart.Equal(base) || !slices.Equal(got.InputEventIDs, []string{"hb-1"}) {
		t.Fatalf("timer feature times/inputs = %s %s %v, want detection %s, window start %s, [hb-1]", got.EventTime, got.WindowStart, got.InputEventIDs, detection, base)
	}
	if !got.TraceContinuation || got.Traceparent != seen.Traceparent {
		t.Fatalf("timer feature trace = %q continuation %v, want the last heartbeat's trace", got.Traceparent, got.TraceContinuation)
	}
}

func TestHeartbeatTimerFiresOnlyOnceTheDeadlineHasPassed(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		at   time.Duration
		want int
	}{
		{"before the deadline", 5*time.Minute - time.Nanosecond, 0},
		{"at the deadline", 5 * time.Minute, 1},
		{"after the deadline", 9 * time.Minute, 1},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			h := newHarness(t, heartbeatSpec())
			h.applyOnTime(heartbeat("hb", base, nil))
			if got := len(h.fireTimer(base.Add(tc.at))); got != tc.want {
				t.Fatalf("timer features = %d, want %d", got, tc.want)
			}
		})
	}
}

func TestHeartbeatTimerMeasuresFromEventTimeWhenProcessingTimeWasNeverRecorded(t *testing.T) {
	t.Parallel()
	h := newHarness(t, heartbeatSpec())
	seen := base
	h.ps = &domain.PartitionState{OperatorStates: map[string]map[string]*domain.OperatorStateBlob{
		"heartbeat_missing": {
			"motor-17": {Heartbeat: &domain.HeartbeatState{LastEventTime: &seen, LastEventID: "legacy"}},
			"motor-18": {},
		},
	}}
	features := h.fireTimer(base.Add(5 * time.Minute))
	if len(features) != 1 || features[0].EntityID != "motor-17" {
		t.Fatalf("timer features = %+v, want one for motor-17 only", features)
	}
}

func TestHeartbeatTimerOrdersFeaturesByStateKey(t *testing.T) {
	t.Parallel()
	h := newHarness(t, heartbeatSpec())
	for _, id := range []string{"motor-30", "motor-02", "motor-11"} {
		env := heartbeat("hb-"+id, base, nil)
		env.Entity.ID = id
		h.applyOnTime(env)
	}
	var got []string
	for _, feature := range h.fireTimer(base.Add(5 * time.Minute)) {
		got = append(got, feature.EntityID)
	}
	if want := []string{"motor-02", "motor-11", "motor-30"}; !slices.Equal(got, want) {
		t.Fatalf("timer order = %v, want %v", got, want)
	}
}

func TestHeartbeatTimerFiresEachOperatorOnceAndSkipsOtherKinds(t *testing.T) {
	t.Parallel()
	compiled := meanSpec()
	compiled.Inputs = append(compiled.Inputs,
		spec.Input{Name: "heartbeat", EventType: "test.heartbeat", SchemaVersion: "1.0", PartitionKey: "entity.id", EntityType: "motor"},
		spec.Input{Name: "heartbeat_b", EventType: "test.heartbeat_b", SchemaVersion: "1.0", PartitionKey: "entity.id", EntityType: "motor"})
	compiled.Operators = append(compiled.Operators, spec.Operator{Name: "alive", Kind: "missing_heartbeat", Inputs: []string{"heartbeat", "heartbeat_b"}, Duration: "5m", Output: "alive_5m"})
	h := newHarness(t, compiled)
	h.applyOnTime(temperature("t", base, 1))
	h.applyOnTime(heartbeat("hb", base, nil))
	features := h.fireTimer(base.Add(5 * time.Minute))
	if len(features) != 1 || features[0].OperatorID != "alive" {
		t.Fatalf("timer features = %+v, want exactly one from the heartbeat operator", features)
	}
}

func TestHeartbeatTimerCarriesExplicitTenantAndPartition(t *testing.T) {
	t.Parallel()
	h := newHarness(t, heartbeatSpec())
	env := heartbeat("hb", base, map[string]any{"boot_id": "boot-A"})
	env.TenantID, env.PartitionKey = "tenant-b", "partition-b"
	h.applyOnTime(env)
	identity := domain.TimerIdentity{TenantID: env.TenantID, PartitionID: env.PartitionID(0)}
	features, _, err := h.rt.ApplyTimer(t.Context(), h.ps, base.Add(5*time.Minute), base.Add(5*time.Minute), identity)
	if err != nil || len(features) != 1 {
		t.Fatalf("timer features = %d err %v, want 1", len(features), err)
	}
	if features[0].TenantID != identity.TenantID || features[0].PartitionID != identity.PartitionID {
		t.Fatalf("timer tenant/partition = %q/%d, want %q/%d", features[0].TenantID, features[0].PartitionID, identity.TenantID, identity.PartitionID)
	}
}

func TestHeartbeatTimerWithoutIdentityLeavesTenantUnknown(t *testing.T) {
	t.Parallel()
	h := newHarness(t, heartbeatSpec())
	h.applyOnTime(heartbeat("hb", base, nil))
	features := h.fireTimer(base.Add(5 * time.Minute))
	if len(features) != 1 || features[0].TenantID != "" || features[0].PartitionID != -1 {
		t.Fatalf("timer feature = %+v, want an unknown tenant and partition -1", features)
	}
}

func TestTimerIdentityIsValidated(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		identities []domain.TimerIdentity
		wantErr    string
	}{
		{"given twice", []domain.TimerIdentity{{TenantID: "a"}, {TenantID: "b"}}, "at most once"},
		{"without tenant", []domain.TimerIdentity{{PartitionID: 1}}, "tenant is required"},
		{"negative partition", []domain.TimerIdentity{{TenantID: "a", PartitionID: -1}}, "partition must be non-negative"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			h := newHarness(t, heartbeatSpec())
			_, _, err := h.rt.ApplyTimer(t.Context(), h.ps, base, base, tc.identities...)
			requireErrorContaining(t, err, tc.wantErr)
		})
	}
}

func TestApplyTimerWithoutPartitionStateDoesNothing(t *testing.T) {
	t.Parallel()
	h := newHarness(t, heartbeatSpec())
	features, ps, err := h.rt.ApplyTimer(t.Context(), nil, base, base)
	if err != nil || features != nil || ps != nil {
		t.Fatalf("ApplyTimer(nil) = %v, %v, %v; want nothing", features, ps, err)
	}
}

func TestApplyTimerHonorsCancellation(t *testing.T) {
	t.Parallel()
	h := newHarness(t, heartbeatSpec())
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	_, _, err := h.rt.ApplyTimer(ctx, &domain.PartitionState{}, time.Time{}, time.Time{})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("ApplyTimer error = %v, want %v", err, context.Canceled)
	}
}

func TestMalformedHeartbeatDurationFailsEventAndTimer(t *testing.T) {
	t.Parallel()
	compiled := heartbeatSpec()
	compiled.Operators[0].Duration = "soon"
	h := newHarness(t, compiled)
	_, _, err := h.rt.ApplyEventAt(t.Context(), h.ps, heartbeat("hb", base, nil), base, base)
	requireErrorContaining(t, err, "heartbeat duration:")

	seen := base
	h.ps = &domain.PartitionState{OperatorStates: map[string]map[string]*domain.OperatorStateBlob{
		"heartbeat_missing": {"motor-17": {Heartbeat: &domain.HeartbeatState{LastEventTime: &seen}}},
	}}
	_, _, err = h.rt.ApplyTimer(t.Context(), h.ps, base, base)
	requireErrorContaining(t, err, "heartbeat duration:")
}

func TestInvalidHeartbeatDoesNotRefreshLiveness(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		data    map[string]any
		quality []contractsv1.QualityFlag
	}{
		{"invalid payload quality", map[string]any{"quality": "invalid"}, nil},
		{"warming payload quality", map[string]any{"quality": "warming"}, nil},
		{"disconnected envelope quality", nil, []contractsv1.QualityFlag{{Code: "disconnected"}}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			h := newHarness(t, heartbeatSpec())
			valid := h.applyOnTime(heartbeat("valid", base, map[string]any{"quality": "valid"}))
			if len(valid) != 1 || valid[0].Value != false {
				t.Fatalf("valid heartbeat feature = %+v, want one false feature", valid)
			}
			invalid := heartbeat("invalid", base.Add(4*time.Minute), tc.data)
			invalid.Quality = tc.quality
			if features := h.applyOnTime(invalid); len(features) != 0 {
				t.Fatalf("invalid heartbeat emitted %d features, want 0", len(features))
			}
			features := h.fireTimer(base.Add(5 * time.Minute))
			if len(features) != 1 || features[0].Value != true || !slices.Equal(features[0].InputEventIDs, []string{"valid"}) {
				t.Fatalf("timer features = %+v, want one missing feature over the valid heartbeat", features)
			}
		})
	}
}
