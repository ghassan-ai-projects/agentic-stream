package domain

import (
	"context"
	"fmt"
	"maps"
	"slices"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/contractsv1"
	"github.com/ghassan-ai-projects/agentic-stream/internal/sources"
)

func (r *OperatorRuntime) applyArrival(ctx context.Context, ps *PartitionState, inputName string, env contractsv1.Envelope, watermark, processingTime time.Time) ([]Feature, error) {
	features, err := r.applyInput(ctx, ps, inputName, env, watermark, processingTime)
	if err != nil {
		return nil, err
	}
	stale, err := r.staleAdvisoryInputs(ps, inputName, env, watermark)
	if err != nil {
		return nil, err
	}
	return append(features, stale...), nil
}

func (r *OperatorRuntime) staleAdvisoryInputs(ps *PartitionState, arrived string, env contractsv1.Envelope, watermark time.Time) ([]Feature, error) {
	var features []Feature
	for _, inst := range r.advisoryHeartbeats(arrived) {
		duration, err := parseDuration(inst.def.Duration)
		if err != nil {
			return nil, fmt.Errorf("advisory heartbeat %s duration: %w", inst.def.Name, err)
		}
		features = append(features, r.newlyStale(ps, inst, env, watermark, duration)...)
	}
	return features, nil
}

func (r *OperatorRuntime) advisoryHeartbeats(arrived string) []*operatorInstance {
	var heartbeats []*operatorInstance
	for _, input := range r.spec.Inputs {
		if !input.Advisory() || input.Name == arrived {
			continue
		}
		for _, inst := range r.byInput[input.Name] {
			if inst.def.Kind == "missing_heartbeat" {
				heartbeats = append(heartbeats, inst)
			}
		}
	}
	return heartbeats
}

func (r *OperatorRuntime) newlyStale(ps *PartitionState, inst *operatorInstance, env contractsv1.Envelope, watermark time.Time, duration time.Duration) []Feature {
	states := ps.OperatorStates[inst.def.Name]
	var features []Feature
	for _, stateKey := range slices.Sorted(maps.Keys(states)) {
		hs := states[stateKey].Heartbeat
		if entityIDFromStateKey(stateKey) != env.Entity.ID || !r.isActiveBoot(ps, stateKey) || !hs.staleAt(watermark, duration) {
			continue
		}
		hs.Stale = true
		features = append(features, r.staleFeature(inst, stateKey, hs, env, watermark, duration))
	}
	return features
}

func (hs *HeartbeatState) staleAt(watermark time.Time, duration time.Duration) bool {
	return hs != nil && hs.LastEventTime != nil && !hs.Stale && watermark.Sub(*hs.LastEventTime) >= duration
}

func (r *OperatorRuntime) staleFeature(inst *operatorInstance, stateKey string, hs *HeartbeatState, env contractsv1.Envelope, watermark time.Time, duration time.Duration) Feature {
	return Feature{
		FeatureID: r.idGen.New(sources.PrefixEvent), OperatorID: inst.def.Name, OutputName: inst.def.Output,
		TenantID: env.TenantID, EntityType: env.Entity.Type, EntityID: env.Entity.ID,
		StateKey: stateKey, BootID: hs.BootID, PartitionID: env.PartitionID(0),
		WindowStart: *hs.LastEventTime, WindowEnd: hs.LastEventTime.Add(duration), Value: true,
		EventTime: hs.LastEventTime.Add(duration), Watermark: watermark, InputEventIDs: []string{hs.LastEventID},
		Completeness: string(CompletenessUncertain), Traceparent: hs.Traceparent, Tracestate: hs.Tracestate,
	}
}
