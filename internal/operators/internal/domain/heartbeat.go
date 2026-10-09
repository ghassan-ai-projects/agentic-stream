package domain

import (
	"context"
	"fmt"
	"maps"
	"slices"
	"strings"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/contractsv1"
	"github.com/ghassan-ai-projects/agentic-stream/internal/sources"
)

func (r *OperatorRuntime) applyHeartbeatOperator(inst *operatorInstance, blob *OperatorStateBlob, env contractsv1.Envelope, watermark, processingTime time.Time) ([]Feature, error) {
	if !sampleQualityValid(env) {
		return nil, nil
	}
	if processingTime.IsZero() {
		processingTime = env.EventTime
	}
	hs := recordHeartbeat(blob, env, processingTime)
	duration, err := parseDuration(inst.def.Duration)
	if err != nil {
		return nil, fmt.Errorf("heartbeat duration: %w", err)
	}
	missing := watermark.Sub(*hs.LastEventTime) >= duration
	return []Feature{r.heartbeatFeature(inst, env, hs, watermark, duration, missing)}, nil
}

func recordHeartbeat(blob *OperatorStateBlob, env contractsv1.Envelope, processingTime time.Time) *HeartbeatState {
	if blob.Heartbeat == nil {
		blob.Heartbeat = &HeartbeatState{}
	}
	hs := blob.Heartbeat
	hs.adoptBoot(deviceBootID(env))
	if hs.isNewerThanLatest(env) {
		hs.LastEventTime = &env.EventTime
		hs.LastEventID = env.ID
		hs.Traceparent = env.Traceparent
		hs.Tracestate = env.Tracestate
	}
	hs.LastProcessingTime = &processingTime
	return hs
}

func (hs *HeartbeatState) isNewerThanLatest(env contractsv1.Envelope) bool {
	if hs.LastEventTime == nil || env.EventTime.After(*hs.LastEventTime) {
		return true
	}
	return env.EventTime.Equal(*hs.LastEventTime) && env.ID > hs.LastEventID
}

func (hs *HeartbeatState) adoptBoot(bootID string) {
	if bootID != "" && hs.BootID != bootID {
		*hs = HeartbeatState{BootID: bootID}
	} else if hs.BootID == "" {
		hs.BootID = bootID
	}
}

func (r *OperatorRuntime) heartbeatFeature(inst *operatorInstance, env contractsv1.Envelope, hs *HeartbeatState, watermark time.Time, duration time.Duration, missing bool) Feature {
	feature := Feature{
		FeatureID: r.idGen.New(sources.PrefixEvent), OperatorID: inst.def.Name, OutputName: inst.def.Output,
		TenantID: env.TenantID, EntityType: env.Entity.Type, EntityID: env.Entity.ID,
		StateKey: operatorStateKey(env), BootID: hs.BootID, PartitionID: env.PartitionID(0),
		WindowStart: *hs.LastEventTime, WindowEnd: watermark, Value: missing,
		EventTime: *hs.LastEventTime, Watermark: watermark, InputEventIDs: []string{hs.LastEventID},
		Completeness: string(CompletenessOnTime), Traceparent: hs.Traceparent, Tracestate: hs.Tracestate,
	}
	if missing {
		feature.EventTime = hs.LastEventTime.Add(duration)
		feature.Completeness = string(CompletenessUncertain)
	}
	return feature
}

type TimerIdentity struct {
	TenantID    string
	PartitionID int
}

func (r *OperatorRuntime) ApplyTimer(ctx context.Context, ps *PartitionState, watermark, processingTime time.Time, identities ...TimerIdentity) ([]Feature, *PartitionState, error) {
	if err := ctx.Err(); err != nil {
		return nil, ps, fmt.Errorf("apply timer canceled: %w", err)
	}
	if ps == nil {
		return nil, ps, nil
	}
	identity, err := timerIdentity(identities)
	if err != nil {
		return nil, ps, err
	}
	features, err := r.applyHeartbeatTimers(ctx, ps, watermark, processingTime, identity)
	return features, ps, err
}

func (r *OperatorRuntime) applyHeartbeatTimers(ctx context.Context, ps *PartitionState, watermark, processingTime time.Time, identity TimerIdentity) ([]Feature, error) {
	var features []Feature
	for _, op := range r.operatorsByName() {
		if op.def.Kind != "missing_heartbeat" {
			continue
		}
		fs, err := r.applyHeartbeatTimer(ctx, op, ps, watermark, processingTime, identity)
		if err != nil {
			return nil, err
		}
		features = append(features, fs...)
	}
	return features, nil
}

func (r *OperatorRuntime) operatorsByName() []*operatorInstance {
	var instances []*operatorInstance
	for _, inputInstances := range r.byInput {
		instances = append(instances, inputInstances...)
	}
	slices.SortStableFunc(instances, func(a, b *operatorInstance) int { return strings.Compare(a.def.Name, b.def.Name) })
	return slices.CompactFunc(instances, func(a, b *operatorInstance) bool { return a.def.Name == b.def.Name })
}

func timerIdentity(identities []TimerIdentity) (TimerIdentity, error) {
	if len(identities) > 1 {
		return TimerIdentity{}, fmt.Errorf("timer identity must be provided at most once")
	}
	if len(identities) == 0 {
		return TimerIdentity{PartitionID: -1}, nil
	}
	identity := identities[0]
	if identity.TenantID == "" {
		return TimerIdentity{}, fmt.Errorf("timer identity tenant is required")
	}
	if identity.PartitionID < 0 {
		return TimerIdentity{}, fmt.Errorf("timer identity partition must be non-negative")
	}
	return identity, nil
}

func (r *OperatorRuntime) applyHeartbeatTimer(ctx context.Context, inst *operatorInstance, ps *PartitionState, watermark, processingTime time.Time, identity TimerIdentity) ([]Feature, error) {
	duration, err := parseDuration(inst.def.Duration)
	if err != nil {
		return nil, fmt.Errorf("heartbeat duration: %w", err)
	}
	var features []Feature
	states := ps.OperatorStates[inst.def.Name]
	for _, stateKey := range slices.Sorted(maps.Keys(states)) {
		if err := ctx.Err(); err != nil {
			return nil, fmt.Errorf("heartbeat timer canceled: %w", err)
		}
		if hs := states[stateKey].Heartbeat; r.isActiveBoot(ps, stateKey) && heartbeatOverdue(hs, processingTime, duration) {
			features = append(features, r.missedHeartbeatFeature(inst, stateKey, hs, identity, watermark, duration))
		}
	}
	return features, nil
}

func heartbeatOverdue(hs *HeartbeatState, processingTime time.Time, duration time.Duration) bool {
	if hs == nil || hs.LastEventTime == nil {
		return false
	}
	lastProcessingTime := hs.LastProcessingTime
	if lastProcessingTime == nil {
		lastProcessingTime = hs.LastEventTime
	}
	return processingTime.Sub(*lastProcessingTime) >= duration
}

func (r *OperatorRuntime) missedHeartbeatFeature(inst *operatorInstance, stateKey string, hs *HeartbeatState, identity TimerIdentity, watermark time.Time, duration time.Duration) Feature {
	return Feature{
		FeatureID: r.idGen.New(sources.PrefixEvent), OperatorID: inst.def.Name, OutputName: inst.def.Output,

		TenantID: identity.TenantID, EntityType: r.entityTypeForOperator(inst.def.Name),
		EntityID: entityIDFromStateKey(stateKey), StateKey: stateKey, BootID: hs.BootID, PartitionID: identity.PartitionID,
		WindowStart: *hs.LastEventTime, WindowEnd: hs.LastEventTime.Add(duration), Value: true,
		EventTime: hs.LastEventTime.Add(duration), Watermark: watermark, InputEventIDs: []string{hs.LastEventID},
		Completeness: string(CompletenessUncertain), TraceContinuation: true,
		Traceparent: hs.Traceparent, Tracestate: hs.Tracestate,
	}
}
