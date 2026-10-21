package domain

func (s *CompiledSpec) DispatchedIntentTypes() []string {
	if EffectiveDispatchPolicy(s.Cognition.Executor.DispatchPolicy) != DispatchActive {
		return nil
	}
	types := make([]string, 0, len(s.Actions.Intents))
	for _, intent := range s.Actions.Intents {
		types = append(types, intent.Type)
	}
	return types
}
