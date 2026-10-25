package wire_test

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/plantwire/internal/wire"
)

var benchSite = filepath.Join("..", "..", "..", "..", "examples", "real-world-sensor", "testdata", "plant-wire", "thermal-bench.site.json")

func TestParseSiteFileReadsTheBenchExample(t *testing.T) {
	t.Parallel()
	raw, err := os.ReadFile(benchSite)
	if err != nil {
		t.Fatal(err)
	}
	site, err := wire.ParseSiteFile(raw)
	if err != nil {
		t.Fatalf("ParseSiteFile = %v", err)
	}
	if site.Version != 1 || len(site.Devices) != 3 || len(site.Readings) != 2 || len(site.Heartbeats) != 1 || len(site.Quality) != 8 {
		t.Fatalf("site = %+v, want three devices, two readings, one heartbeat and eight quality codes", site)
	}
	if beat := site.Heartbeats[0]; beat.EveryMS != 10_000 || beat.StaleAfterMS != 6_000 || beat.EventType == "" || site.Devices["zone-01"].Entity == nil {
		t.Fatalf("site = %+v, want the zone's heartbeat every 10 s and its entity", site)
	}
}

func TestParseSiteFileRefusesWhatIsNotASiteFile(t *testing.T) {
	t.Parallel()
	raw, err := os.ReadFile(benchSite)
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name    string
		raw     string
		members []string
	}{
		{"a heartbeat every zero milliseconds", strings.Replace(string(raw), `"every_ms": 10000`, `"every_ms": 0`, 1), []string{"/heartbeats/0/every_ms"}},
		{"a quality code missing", strings.Replace(string(raw), `"stale": "invalid",`, "", 1), []string{"/quality/stale"}},
		{"an unknown member", strings.Replace(string(raw), `"version": 1,`, `"version": 1, "tenant": "x",`, 1), []string{"/tenant"}},
		{"a device id with a space", strings.Replace(string(raw), `"led-01"`, `"led 01"`, 1), []string{""}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := wire.ParseSiteFile([]byte(tc.raw))
			var refused *wire.SchemaError
			if !errors.As(err, &refused) || !slices.Equal(refused.Members, tc.members) {
				t.Fatalf("ParseSiteFile = %v, want a schema refusal at %v", err, tc.members)
			}
		})
	}
	if _, err := wire.ParseSiteFile([]byte("[]")); !errors.Is(err, wire.ErrNotARecord) {
		t.Fatalf("ParseSiteFile([]) = %v, want ErrNotARecord", err)
	}
}
