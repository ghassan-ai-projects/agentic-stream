package operators

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/contractsv1"
	"github.com/ghassan-ai-projects/agentic-stream/internal/ids"
)

func (r *OperatorRuntime) applyHeartbeatOperator(inst *operatorInstance, blob *OperatorStateBlob, env contractsv1.Envelope, watermark, processingTime time.Time) ([]Feature, error) {
	if !sampleQualityValid(env) {
		return nil, nil
	}

	if blob.Heartbeat == nil {
		blob.Heartbeat = &HeartbeatState{}
	}
	hs := blob.Heartbeat
	bootID := deviceBootID(env)
	if bootID != "" && hs.BootID != bootID {
		*hs = HeartbeatState{BootID: bootID}
	} else if hs.BootID == "" {
		hs.BootID = bootID
	}
	hs.LastEventTime = &env.EventTime
	hs.LastEventID = env.ID
	hs.Traceparent = env.Traceparent
	hs.Tracestate = env.Tracestate
	if processingTime.IsZero() {
		processingTime = env.EventTime
	}
	hs.LastProcessingTime = &processingTime

	duration, err := parseDuration(inst.def.Duration)
	if err != nil {
		return nil, fmt.Errorf("heartbeat duration: %w", err)
	}

	missing := watermark.Sub(*hs.LastEventTime) >= duration
	eventTime := env.EventTime
	if missing {
		eventTime = processingTime
	}

	feature := Feature{
		FeatureID:     r.idGen.New(ids.PrefixEvent),
		OperatorID:    inst.def.Name,
		OutputName:    inst.def.Output,
		TenantID:      env.TenantID,
		EntityType:    env.Entity.Type,
		EntityID:      env.Entity.ID,
		StateKey:      operatorStateKey(env),
		BootID:        bootID,
		PartitionID:   env.PartitionID(0),
		WindowStart:   env.EventTime,
		WindowEnd:     watermark,
		Value:         missing,
		EventTime:     eventTime,
		Watermark:     watermark,
		InputEventIDs: []string{env.ID},
		Completeness:  string(CompletenessOnTime),
		Traceparent:   env.Traceparent,
		Tracestate:    env.Tracestate,
	}
	if missing {
		feature.Completeness = string(CompletenessUncertain)
	}
	return []Feature{feature}, nil
}

type TimerIdentity struct {
	TenantID    string
	PartitionID int
}

// ApplyTimer fires due timers and emits any resulting features. In Phase 2 this
// is used primarily for missing-heartbeat detection. The optional identity is
// required by direct callers that consume the returned feature; the engine's
// persistence path supplies its authoritative tenant and partition while
// enriching timer features.
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

	var features []Feature
	var instances []*operatorInstance
	for _, inputInstances := range r.byInput {
		instances = append(instances, inputInstances...)
	}
	slices.SortStableFunc(instances, func(a, b *operatorInstance) int {
		return strings.Compare(a.def.Name, b.def.Name)
	})
	seen := make(map[string]struct{}, len(instances))
	for _, op := range instances {
		if _, ok := seen[op.def.Name]; ok {
			continue
		}
		seen[op.def.Name] = struct{}{}
		if op.def.Kind != "missing_heartbeat" {
			continue
		}
		fs, err := r.applyHeartbeatTimer(ctx, op, ps, watermark, processingTime, identity)
		if err != nil {
			return nil, ps, err
		}
		features = append(features, fs...)
	}
	return features, ps, nil
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
	keys := make([]string, 0, len(ps.OperatorStates[inst.def.Name]))
	for stateKey := range ps.OperatorStates[inst.def.Name] {
		keys = append(keys, stateKey)
	}
	slices.Sort(keys)
	for _, stateKey := range keys {
		if err := ctx.Err(); err != nil {
			return nil, fmt.Errorf("heartbeat timer canceled: %w", err)
		}
		if !r.isActiveBoot(ps, stateKey) {
			continue
		}
		blob := ps.OperatorStates[inst.def.Name][stateKey]
		if blob.Heartbeat == nil || blob.Heartbeat.LastEventTime == nil {
			continue
		}
		lastProcessingTime := blob.Heartbeat.LastProcessingTime
		if lastProcessingTime == nil {
			lastProcessingTime = blob.Heartbeat.LastEventTime
		}
		missing := processingTime.Sub(*lastProcessingTime) >= duration
		if !missing {
			continue
		}
		// Determine entity type/id from stateKey. For Phase 2 entity type is
		// known from the spec input.
		entityType := r.entityTypeForOperator(inst.def.Name)
		features = append(features, Feature{
			FeatureID:         r.idGen.New(ids.PrefixEvent),
			OperatorID:        inst.def.Name,
			OutputName:        inst.def.Output,
			TenantID:          identity.TenantID,
			EntityType:        entityType,
			EntityID:          entityIDFromStateKey(stateKey),
			StateKey:          stateKey,
			BootID:            blob.Heartbeat.BootID,
			PartitionID:       identity.PartitionID,
			WindowStart:       *blob.Heartbeat.LastEventTime,
			WindowEnd:         processingTime,
			Value:             true,
			EventTime:         processingTime,
			Watermark:         watermark,
			InputEventIDs:     []string{blob.Heartbeat.LastEventID},
			Completeness:      string(CompletenessUncertain),
			TraceContinuation: true,
			Traceparent:       blob.Heartbeat.Traceparent,
			Tracestate:        blob.Heartbeat.Tracestate,
		})
	}
	return features, nil
}
