package domain

var completenessRank = map[Completeness]int{
	CompletenessUncertain:     0,
	CompletenessProvisional:   1,
	CompletenessOnTime:        2,
	CompletenessCorrected:     3,
	CompletenessFinalByPolicy: 4,
}

func (c Completeness) AtLeast(required Completeness) bool {
	return completenessRank[c] >= completenessRank[required]
}

func Weakest(values []Completeness) Completeness {
	if len(values) == 0 {
		return ""
	}
	weakest := values[0]
	for _, value := range values[1:] {
		if completenessRank[value] < completenessRank[weakest] {
			weakest = value
		}
	}
	return weakest
}
