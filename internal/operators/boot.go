package operators

import (
	"github.com/ghassan-ai-projects/agentic-stream/internal/contractsv1"
	"github.com/ghassan-ai-projects/agentic-stream/internal/eventschema"
	"strings"
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
	stateKey := env.Entity.ID
	meta := r.getBlob(ps, RuntimeOperatorID, stateKey)
	if meta.Runtime == nil {
		meta.Runtime = &RuntimeState{}
	}
	runtimeState := meta.Runtime
	bootID := deviceBootID(env)
	if bootID == "" {
		return runtimeState.CurrentBootID == ""
	}
	if runtimeState.CurrentBootID == bootID {
		return true
	}
	for _, seen := range runtimeState.SeenBootIDs {
		if seen == bootID {
			return false
		}
	}
	if len(runtimeState.SeenBootIDs) >= maxSeenBootIDs {
		// A bounded history cannot safely distinguish an evicted old boot from
		// a new one. Fail closed rather than allowing stale evidence to revive.
		return false
	}
	for operatorID, states := range ps.OperatorStates {
		if operatorID == RuntimeOperatorID {
			continue
		}
		for stateKey := range states {
			if stateKey == env.Entity.ID || strings.HasPrefix(stateKey, env.Entity.ID+"\x1f") {
				// Only the active boot's state is useful after admission. The
				// tombstone list above still prevents a retired boot from being
				// admitted again.
				delete(states, stateKey)
			}
		}
	}
	runtimeState.CurrentBootID = bootID
	runtimeState.SeenBootIDs = append(runtimeState.SeenBootIDs, bootID)
	return true
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
