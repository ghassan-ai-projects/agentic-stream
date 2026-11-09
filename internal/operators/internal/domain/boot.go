package domain

import (
	"slices"
	"strings"

	"github.com/ghassan-ai-projects/agentic-stream/internal/contractsv1"
)

const maxSeenBootIDs = 64

func (r *OperatorRuntime) inputRequiresBootIdentity(inst *operatorInstance) bool {
	for _, inputName := range inst.def.Inputs {
		for _, input := range r.spec.Inputs {
			if input.Name != inputName || input.SchemaRef == "" {
				continue
			}
			definition, ok := r.spec.EventSchema(input.SchemaRef)
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

func (r *OperatorRuntime) admitBoot(ps *PartitionState, env contractsv1.Envelope) bool {
	fence := r.runtimeState(ps, env.Entity.ID).fenceOf(env.Type)
	bootID := deviceBootID(env)
	switch {
	case bootID == "":
		return fence.CurrentBootID == ""
	case fence.CurrentBootID == bootID:
		return true
	case !fence.acceptsNewBoot(bootID):
		return false
	}
	r.discardStateFedBy(ps, env.Entity.ID, env.Type)
	fence.CurrentBootID = bootID
	fence.SeenBootIDs = append(fence.SeenBootIDs, bootID)
	return true
}

func (r *OperatorRuntime) runtimeState(ps *PartitionState, entityID string) *RuntimeState {
	meta := r.getBlob(ps, RuntimeOperatorID, entityID)
	if meta.Runtime == nil {
		meta.Runtime = &RuntimeState{}
	}
	return meta.Runtime
}

func (s *RuntimeState) fenceOf(eventType string) *BootFence {
	if s.Fences == nil {
		s.Fences = map[string]*BootFence{}
	}
	fence, fenced := s.Fences[eventType]
	if !fenced {
		fence = &BootFence{}
		s.Fences[eventType] = fence
	}
	return fence
}

func (f *BootFence) acceptsNewBoot(bootID string) bool {
	return !slices.Contains(f.SeenBootIDs, bootID) && len(f.SeenBootIDs) < maxSeenBootIDs
}

func (r *OperatorRuntime) discardStateFedBy(ps *PartitionState, entityID, eventType string) {
	for _, input := range r.spec.Inputs {
		if input.EventType != eventType {
			continue
		}
		for _, inst := range r.byInput[input.Name] {
			discardEntityState(ps.OperatorStates[inst.def.Name], entityID)
		}
	}
}

func discardEntityState(states map[string]*OperatorStateBlob, entityID string) {
	for stateKey := range states {
		if stateKey == entityID || strings.HasPrefix(stateKey, entityID+"\x1f") {
			delete(states, stateKey)
		}
	}
}

func (r *OperatorRuntime) isActiveBoot(ps *PartitionState, stateKey string) bool {
	meta := ps.OperatorStates[RuntimeOperatorID][entityIDFromStateKey(stateKey)]
	if meta == nil || meta.Runtime == nil {
		return true
	}
	return meta.Runtime.admits(bootIDFromStateKey(stateKey))
}

func (s *RuntimeState) admits(bootID string) bool {
	current := s.currentBoots()
	return len(current) == 0 || bootID != "" && slices.Contains(current, bootID)
}

func (s *RuntimeState) currentBoots() []string {
	var boots []string
	for _, fence := range s.Fences {
		if fence.CurrentBootID != "" {
			boots = append(boots, fence.CurrentBootID)
		}
	}
	return boots
}

func (r *OperatorRuntime) IsTimerStateActive(ps *PartitionState, stateKey string) bool {
	if ps == nil {
		return false
	}
	return r.isActiveBoot(ps, stateKey)
}
