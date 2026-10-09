package domain

import (
	"encoding/json"
	"testing"
)

func TestInstalledWatchIDReadsWhatInstallResultWrites(t *testing.T) {
	t.Parallel()
	encoded, err := json.Marshal(InstallResult("watch-7"))
	if err != nil {
		t.Fatal(err)
	}
	if got := InstalledWatchID(encoded); got != "watch-7" {
		t.Fatalf("InstalledWatchID = %q, want watch-7", got)
	}
}

func TestInstalledWatchIDIgnoresResultsWithoutAWatch(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct{ name, result string }{
		{"empty", ""},
		{"not json", "accepted"},
		{"no watch id", `{"accepted":true}`},
		{"wrong type", `{"watch_id":7}`},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := InstalledWatchID([]byte(tt.result)); got != "" {
				t.Fatalf("InstalledWatchID(%q) = %q, want empty", tt.result, got)
			}
		})
	}
}
