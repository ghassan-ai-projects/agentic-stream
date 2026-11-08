package domain_test

import (
	"errors"
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/contractsv1"
	"github.com/ghassan-ai-projects/agentic-stream/internal/plantwire/internal/domain"
)

func onDevice(reading domain.Reading, device, eventType string) domain.Reading {
	reading.Device, reading.EventType = device, eventType
	return reading
}

func threeValveSite() domain.Site {
	site := valveSite()
	site.Devices["V2"] = domain.SiteDevice{Kind: "valve", Entity: &contractsv1.EntityRef{Type: "line", ID: "line-1"}}
	site.Readings = append(site.Readings,
		onDevice(site.Readings[0], "V2", "line.opening.v2.observed"),
		onDevice(site.Readings[0], "V1", "line.opening.v1.observed"))
	site.Heartbeats = nil
	return site
}

func TestDevicesOfOneKindReadThroughTheirOwnRulesBeforeTheKindRule(t *testing.T) {
	t.Parallel()
	conversation := ingestingSite(t, threeValveSite(), valveInventory(t))
	play(t, conversation, []step{
		{arrival: deviceBirth(1, "V1", "valve", "b1", startNS)},
		{arrival: deviceBirth(2, "V2", "valve", "b1", startNS)},
		{arrival: deviceBirth(3, "V9", "valve", "b1", startNS)},
		{arrival: data(4, "V1", "b1", false, sample(startNS+seconds, "good")), events: []string{"line.opening.v1.observed@01"}},
		{arrival: data(5, "V2", "b1", false, sample(startNS+seconds, "good")), events: []string{"line.opening.v2.observed@01"}},
		{arrival: data(6, "V9", "b1", false, sample(startNS+seconds, "good")), events: []string{"line.opening.observed@01"}},
	})
	if tally := conversation.Tally(); tally != (domain.Tally{Events: 3}) {
		t.Fatalf("Tally = %+v, want one event of its own type per device", tally)
	}
}

func TestNewMappingRefusesADeviceReadingItCannotBind(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name   string
		change func(*domain.Site)
		want   error
	}{
		{"a device the site does not list", func(s *domain.Site) {
			s.Readings = append(s.Readings, onDevice(s.Readings[0], "V5", "line.opening.v5.observed"))
		}, domain.ErrSiteReadingDeviceAbsent},
		{"a device of another kind", func(s *domain.Site) {
			s.Devices["G1"] = domain.SiteDevice{Kind: "gate", Entity: &contractsv1.EntityRef{Type: "line", ID: "line-1"}}
			s.Readings = append(s.Readings, onDevice(s.Readings[0], "G1", "line.opening.g1.observed"))
		}, domain.ErrSiteReadingDeviceKind},
		{"two rules for one device reading", func(s *domain.Site) { s.Readings = append(s.Readings, s.Readings[len(s.Readings)-1]) },
			domain.ErrSiteRuleTwice},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			site := threeValveSite()
			tc.change(&site)
			if _, err := domain.NewMapping(site, valveAndGateInventory(t)); !errors.Is(err, tc.want) {
				t.Fatalf("NewMapping = %v, want %v", err, tc.want)
			}
		})
	}
}

func TestNewMappingAcceptsADeviceRuleBesideTheKindRule(t *testing.T) {
	t.Parallel()
	if _, err := domain.NewMapping(threeValveSite(), valveInventory(t)); err != nil {
		t.Fatalf("NewMapping = %v, want device rules beside the kind rule accepted", err)
	}
}

func valveAndGateInventory(t *testing.T) domain.Inventory {
	t.Helper()
	gate := valveKind()
	gate["kind"] = "gate"
	inventory, err := domain.Commission([]domain.Kind{valveKind(), gate}, nil)
	if err != nil {
		t.Fatal(err)
	}
	return inventory
}
