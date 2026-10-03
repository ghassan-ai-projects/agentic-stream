package operators

import (
	"slices"
	"strings"

	"github.com/ghassan-ai-projects/agentic-stream/internal/contractsv1"
	"github.com/ghassan-ai-projects/agentic-stream/internal/eventschema"
)

const maxSeenBootIDs = 64

func (r *OperatorRuntime) inputRequiresBootIdentity(inst *operatorInstance) bool {
	for _, inputName := range inst.def.Inputs {
		for _, input := range r.spec.Inputs {
			if input.Name != inputName || input.SchemaRef == "" {
				continue
			}
			definition, ok := eventschema.Lookup(input.SchemaRef)
			if !ok {
				return false
			}
			_, hasQuality := definition.Fields["quality"]
			_, hasBootID := definition.Fields["boot_id"]
			return hasQuality && hasBootID
		}
	}
	return false
}

func deviceBootID(env contractsv1.Envelope) string {
	bootID, ok := env.Data["boot_id"].(string)
	if !ok {
		return ""
	}
	return strings.TrimSpace(bootID)
}

func operatorStateKey(env contractsv1.Envelope) string {
	bootID := deviceBootID(env)
	if bootID == "" {
		return env.Entity.ID
	}
	// Device sequence and monotonic time are meaningful only within a boot.
	// Keep explicit boots in separate durable state keys so a delayed event from
	// an older boot cannot reset or contaminate the current boot's window.
	return env.Entity.ID + "\x1f" + bootID
}

func entityIDFromStateKey(stateKey string) string {
	if entityID, _, ok := strings.Cut(stateKey, "\x1f"); ok {
		return entityID
	}
	return stateKey
}

func bootIDFromStateKey(stateKey string) string {
	_, bootID, ok := strings.Cut(stateKey, "\x1f")
	if !ok {
		return ""
	}
	return bootID
}

// admitBoot fences delayed evidence from a prior device boot. Missing boot
// identity remains compatible only until the first identified boot is seen;
// thereafter it cannot be used to mutate physical numeric state.
func (r *OperatorRuntime) admitBoot(ps *PartitionState, env contractsv1.Envelope) bool {
	runtimeState := r.runtimeState(ps, env.Entity.ID)
	bootID := deviceBootID(env)
	if bootID == "" {
		return runtimeState.CurrentBootID == ""
	}
	if runtimeState.CurrentBootID == bootID {
		return true
	}
	if !runtimeState.acceptsNewBoot(bootID) {
		return false
	}
	discardEntityState(ps, env.Entity.ID)
	runtimeState.CurrentBootID = bootID
	runtimeState.SeenBootIDs = append(runtimeState.SeenBootIDs, bootID)
	return true
}

func (r *OperatorRuntime) runtimeState(ps *PartitionState, entityID string) *RuntimeState {
	meta := r.getBlob(ps, RuntimeOperatorID, entityID)
	if meta.Runtime == nil {
		meta.Runtime = &RuntimeState{}
	}
	return meta.Runtime
}

// acceptsNewBoot refuses a boot seen before. A full bounded history cannot
// safely distinguish an evicted old boot from a new one, so it fails closed
// rather than allowing stale evidence to revive.
func (s *RuntimeState) acceptsNewBoot(bootID string) bool {
	return !slices.Contains(s.SeenBootIDs, bootID) && len(s.SeenBootIDs) < maxSeenBootIDs
}

// discardEntityState drops every operator's state for the entity when a new
// boot is admitted. Only the active boot's state is useful afterwards; the
// seen-boot list still prevents a retired boot from being admitted again.
func discardEntityState(ps *PartitionState, entityID string) {
	for operatorID, states := range ps.OperatorStates {
		if operatorID == RuntimeOperatorID {
			continue
		}
		for stateKey := range states {
			if stateKey == entityID || strings.HasPrefix(stateKey, entityID+"\x1f") {
				delete(states, stateKey)
			}
		}
	}
}

func (r *OperatorRuntime) isActiveBoot(ps *PartitionState, stateKey string) bool {
	bootID := bootIDFromStateKey(stateKey)
	meta := ps.OperatorStates[RuntimeOperatorID][entityIDFromStateKey(stateKey)]
	if bootID == "" {
		return meta == nil || meta.Runtime == nil || meta.Runtime.CurrentBootID == ""
	}
	return meta == nil || meta.Runtime == nil || meta.Runtime.CurrentBootID == "" || meta.Runtime.CurrentBootID == bootID
}

// IsTimerStateActive reports whether a persisted timer belongs to the current
// boot admission state. The engine uses this to retire stale timers instead of
// treating their intentional suppression as a processing failure.
func (r *OperatorRuntime) IsTimerStateActive(ps *PartitionState, stateKey string) bool {
	if ps == nil {
		return false
	}
	return r.isActiveBoot(ps, stateKey)
}
