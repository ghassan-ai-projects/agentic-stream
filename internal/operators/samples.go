package operators

import (
	"encoding/json"
	"github.com/ghassan-ai-projects/agentic-stream/internal/contractsv1"
	"math"
	"strings"
)

func (r *OperatorRuntime) extractValue(inst *operatorInstance, env contractsv1.Envelope) (float64, bool) {
	if inst.def.Field == "" || !r.numericObservationQualityValid(inst, env) {
		return 0, false
	}
	parts := strings.Split(inst.def.Field, ".")
	if len(parts) != 2 || parts[0] != "data" {
		return 0, false
	}
	v, ok := env.Data[parts[1]]
	if !ok {
		return 0, false
	}
	switch x := v.(type) {
	case float64:
		return finiteValue(x)
	case float32:
		return finiteValue(float64(x))
	case int:
		return finiteValue(float64(x))
	case int64:
		return finiteValue(float64(x))
	case json.Number:
		f, err := x.Float64()
		if err != nil {
			return 0, false
		}
		return finiteValue(f)
	}
	return 0, false
}

func finiteValue(value float64) (float64, bool) {
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return 0, false
	}
	return value, true
}

// sampleQualityValid accepts legacy payloads that do not carry a quality
// field, but every supplied sample quality must explicitly be valid. Quality
// flags on the envelope can independently invalidate a numeric sample.
func sampleQualityValid(env contractsv1.Envelope) bool {
	if raw, exists := env.Data["quality"]; exists {
		quality, ok := raw.(string)
		if !ok || quality != "valid" {
			return false
		}
	}

	for _, flag := range env.Quality {
		switch strings.ToLower(strings.TrimSpace(flag.Code)) {
		case "warming", "invalid", "disconnected", "rail_high", "rail_low":
			return false
		default:
			// Unknown quality flags are not safe to interpret optimistically.
			return false
		}
	}
	return true
}

func (r *OperatorRuntime) operatorAdmitsEvent(inst *operatorInstance, env contractsv1.Envelope) bool {
	switch inst.def.Kind {
	case "missing_heartbeat":
		return sampleQualityValid(env)
	case "aggregate", "slope":
		return r.numericObservationQualityValid(inst, env)
	default:
		return true
	}
}

// numericObservationQualityValid applies the provenance contract declared by
// the input's registered schema. A schema that exposes both quality and boot
// identity requires a valid quality and an identified boot; no event family is
// special-cased in the runtime.
func (r *OperatorRuntime) numericObservationQualityValid(inst *operatorInstance, env contractsv1.Envelope) bool {
	if !sampleQualityValid(env) {
		return false
	}
	if !r.inputRequiresBootIdentity(inst) {
		return true
	}
	return deviceBootID(env) != ""
}
