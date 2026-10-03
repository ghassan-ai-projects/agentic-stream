package spec

// normalizeSpec copies the raw spec and fills defaults so semantically equivalent
// specs produce the same digest.
func normalizeSpec(r *rawSpec) *CompiledSpec {
	spec := &CompiledSpec{
		SchemaVersion: r.APIVersion,
		Metadata:      r.Metadata,
		Inputs:        r.Inputs,
		Time:          r.Time,
		Windows:       r.Windows,
		Operators:     r.Operators,
		Situation:     r.Situation,
		Cognition:     r.Cognition,
		Actions:       r.Actions,
	}
	defaultInputs(spec.Inputs)
	defaultWindows(spec.Windows)
	defaultCognition(&spec.Cognition)
	defaultIntents(spec.Actions.Intents)
	return spec
}

func defaultInputs(inputs []Input) {
	for i := range inputs {
		if inputs[i].Classification == "" {
			inputs[i].Classification = "internal"
		}
		if inputs[i].MaxPayloadBytes == 0 {
			inputs[i].MaxPayloadBytes = 1048576
		}
	}
}

func defaultWindows(windows []Window) {
	for i := range windows {
		if windows[i].Emit == "" {
			windows[i].Emit = "on_close"
		}
	}
}

func defaultCognition(cognition *Cognition) {
	for i := range cognition.Triggers {
		if cognition.Triggers[i].Completeness == "" {
			cognition.Triggers[i].Completeness = "any"
		}
	}
	if cognition.Executor.RiskCeiling == "" {
		cognition.Executor.RiskCeiling = "R1"
	}
	// P8: the default dispatch policy is SHADOW — nothing enters action
	// governance until the owner declares active. The value rides the
	// compiled digest, so a mode change is a new spec version.
	if cognition.Executor.DispatchPolicy == "" {
		cognition.Executor.DispatchPolicy = "shadow"
	}
}

func defaultIntents(intents []Intent) {
	for i := range intents {
		if intents[i].Policy == "" {
			intents[i].Policy = "approval"
		}
	}
}
