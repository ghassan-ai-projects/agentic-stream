package domain_test

import (
	"errors"
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/contractsv1"
	"github.com/ghassan-ai-projects/agentic-stream/internal/plantwire/internal/domain"
)

func valveSite() domain.Site {
	return domain.Site{
		Version: 1,
		Source:  "plantwire-test",
		Devices: map[string]domain.SiteDevice{
			"V1": {Kind: "valve", Entity: &contractsv1.EntityRef{Type: "line", ID: "line-1"}},
			"V9": {Kind: "valve", Entity: &contractsv1.EntityRef{Type: "line", ID: "line-9"}},
		},
		Readings: []domain.Reading{{Kind: "valve", Metric: "opening", Field: "percent",
			Emitted: domain.Emitted{EventType: "line.opening.observed", SchemaVersion: "1.0"}}},
		Heartbeats: []domain.HeartbeatRule{{Kind: "valve", EveryMS: 10_000, StaleAfterMS: 6_000,
			Emitted: domain.Emitted{EventType: "line.heartbeat.observed", SchemaVersion: "1.0"}}},
		Quality: map[string]string{"good": "valid", "warming_up": "warming", "suspect": "invalid", "out_of_range": "invalid",
			"stale": "invalid", "disconnected": "disconnected", "manual_override": "invalid", "out_of_service": "disconnected"},
	}
}

func TestNewMappingRefusesASiteItCannotServe(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name   string
		change func(*domain.Site)
		want   error
	}{
		{"a non-good quality mapped like good", func(s *domain.Site) { s.Quality["stale"] = "valid" }, domain.ErrSiteQualityLikeGood},
		{"a device of a kind no file commissions", func(s *domain.Site) { s.Devices["P1"] = domain.SiteDevice{Kind: "pump"} },
			domain.ErrSiteKindNotCommissioned},
		{"an emitting device without an entity", func(s *domain.Site) { s.Devices["V2"] = domain.SiteDevice{Kind: "valve"} },
			domain.ErrSiteDeviceWithoutEntity},
		{"a metric the kind does not declare", func(s *domain.Site) { s.Readings[0].Metric = "pressure" }, domain.ErrSiteMetricNotDeclared},
		{"two rules for one reading", func(s *domain.Site) { s.Readings = append(s.Readings, s.Readings[0]) }, domain.ErrSiteRuleTwice},
		{"two heartbeats for one kind", func(s *domain.Site) { s.Heartbeats = append(s.Heartbeats, s.Heartbeats[0]) }, domain.ErrSiteRuleTwice},
		{"a heartbeat of a kind no file commissions", func(s *domain.Site) { s.Heartbeats[0].Kind = "pump" }, domain.ErrSiteKindNotCommissioned},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			site := valveSite()
			tc.change(&site)
			if _, err := domain.NewMapping(site, valveInventory(t)); !errors.Is(err, tc.want) {
				t.Fatalf("NewMapping = %v, want %v", err, tc.want)
			}
		})
	}
}

func TestNewMappingAcceptsADeviceThatGivesNoEventsWithoutAnEntity(t *testing.T) {
	t.Parallel()
	site := valveSite()
	site.Readings, site.Heartbeats = nil, nil
	site.Devices["V2"] = domain.SiteDevice{Kind: "Valve"}
	if _, err := domain.NewMapping(site, valveInventory(t)); err != nil {
		t.Fatalf("NewMapping = %v, want a device without events accepted without an entity", err)
	}
}

func valveInventory(t *testing.T) domain.Inventory {
	t.Helper()
	inventory, err := domain.Commission([]domain.Kind{valveKind()}, nil)
	if err != nil {
		t.Fatal(err)
	}
	return inventory
}
