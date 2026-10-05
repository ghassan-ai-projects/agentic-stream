package domain

// Result is the deterministic output of a replay run.
type Result struct {
	EventsProcessed   int
	VersionCount      int
	VersionsHash      string
	Mode              Mode
	WorkerInvoked     bool
	EffectsAllowed    bool
	CapabilityCalls   int
	SimulatedResults  []map[string]any
	ShadowComparisons []ShadowComparisonResult
	Findings          []Finding
}

// Finding is a deterministic, non-effectful replay observation.
type Finding struct {
	Code    string
	Message string
}

// AllHashesEqual reports whether every result has the same VersionsHash.
func AllHashesEqual(results []Result) bool {
	if len(results) == 0 {
		return true
	}
	first := results[0].VersionsHash
	for _, r := range results[1:] {
		if r.VersionsHash != first {
			return false
		}
	}
	return true
}
