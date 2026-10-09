package domain

import "testing"

func TestOnlyAReconsiderationReportsALiveEpisodeConflict(t *testing.T) {
	t.Parallel()
	for kind, want := range map[string]bool{KindReconsider: true, KindStandard: false, "": false} {
		if got := (Admission{Kind: kind}).ReportsLiveConflict(); got != want {
			t.Errorf("kind %q: ReportsLiveConflict = %v, want %v", kind, got, want)
		}
	}
}
