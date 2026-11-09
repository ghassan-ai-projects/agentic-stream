package campaign

import (
	"cmp"
	"slices"
)

func withSymptomsFolded(placed []detection) []detection {
	firstSymptom, labelled := symptomsOf(placed)
	folded := []detection{}
	for _, d := range placed {
		if kept, keep := foldSymptom(d, firstSymptom, labelled); keep {
			folded = append(folded, kept)
		}
	}
	slices.SortStableFunc(folded, func(a, b detection) int {
		return cmp.Or(cmp.Compare(a.DetectedAt, b.DetectedAt), cmp.Compare(a.EntityID, b.EntityID))
	})
	return folded
}

func foldSymptom(d detection, firstSymptom map[string]string, labelled map[string]bool) (detection, bool) {
	symptom, hasSymptom := firstSymptom[d.EntityID]
	switch {
	case d.Label != "":
		if hasSymptom && symptom < d.DetectedAt {
			d.DetectedAt = symptom
		}
		return d, true
	case !labelled[d.EntityID] && d.DetectedAt == symptom:
		labelled[d.EntityID] = true
		return d, true
	}
	return d, false
}

func symptomsOf(placed []detection) (map[string]string, map[string]bool) {
	firstSymptom, labelled := map[string]string{}, map[string]bool{}
	for _, d := range placed {
		if d.Label != "" {
			labelled[d.EntityID] = true
			continue
		}
		if at, seen := firstSymptom[d.EntityID]; !seen || d.DetectedAt < at {
			firstSymptom[d.EntityID] = d.DetectedAt
		}
	}
	return firstSymptom, labelled
}
