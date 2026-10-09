package domain

const (
	DispatchShadow = "shadow"
	DispatchActive = "active"
	LaneFast       = "fast"
	LaneDeep       = "deep"
)

func EffectiveDispatchPolicy(policy string) string {
	if policy == "" {
		return DispatchShadow
	}
	return policy
}
