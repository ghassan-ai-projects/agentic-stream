package campaign

import (
	"cmp"
	"slices"
)

type symptomKey struct {
	instance, diagnosis string
}

func (s Subjects) withSymptomsFolded(placed []detection) []detection {
	firstSymptom := symptomsOf(placed)
	refined := s.refinedSymptoms(placed)
	reported := map[symptomKey]bool{}
	folded := []detection{}
	for _, d := range placed {
		if kept, keep := s.foldSymptom(d, firstSymptom, refined, reported); keep {
			folded = append(folded, kept)
		}
	}
	slices.SortStableFunc(folded, func(a, b detection) int {
		return cmp.Or(cmp.Compare(a.DetectedAt, b.DetectedAt), cmp.Compare(a.EntityID, b.EntityID))
	})
	return folded
}

func (s Subjects) foldSymptom(d detection, firstSymptom map[symptomKey]string, refined, reported map[symptomKey]bool) (detection, bool) {
	if d.Label != "" {
		return s.datedBySymptom(d, firstSymptom), true
	}
	key := symptomKey{d.EntityID, d.diagnosis}
	if refined[key] || reported[key] || d.DetectedAt != firstSymptom[key] {
		return d, false
	}
	reported[key] = true
	return d, true
}

func (s Subjects) datedBySymptom(d detection, firstSymptom map[symptomKey]string) detection {
	symptom, refines := s.Refines[d.diagnosis]
	if !refines {
		return d
	}
	if at, seen := firstSymptom[symptomKey{d.EntityID, symptom}]; seen && at < d.DetectedAt {
		d.DetectedAt = at
	}
	return d
}

func (s Subjects) refinedSymptoms(placed []detection) map[symptomKey]bool {
	refined := map[symptomKey]bool{}
	for _, d := range placed {
		if symptom, refines := s.Refines[d.diagnosis]; refines && d.Label != "" {
			refined[symptomKey{d.EntityID, symptom}] = true
		}
	}
	return refined
}

func symptomsOf(placed []detection) map[symptomKey]string {
	firstSymptom := map[symptomKey]string{}
	for _, d := range placed {
		if d.Label != "" {
			continue
		}
		key := symptomKey{d.EntityID, d.diagnosis}
		if at, seen := firstSymptom[key]; !seen || d.DetectedAt < at {
			firstSymptom[key] = d.DetectedAt
		}
	}
	return firstSymptom
}
