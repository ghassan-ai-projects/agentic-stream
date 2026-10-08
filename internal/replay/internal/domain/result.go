package domain

type Request struct {
	DBPath    string
	SpecPath  string
	TracePath string
	TenantID  string
}

type Result struct {
	EventsProcessed   int
	VersionCount      int
	VersionsHash      string
	Mode              Mode
	WorkerInvoked     bool
	EffectsAllowed    bool
	CapabilityCalls   int
	ShadowComparisons []ShadowComparisonResult
	Findings          []Finding
}

type Finding struct {
	Code    string
	Message string
}

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
