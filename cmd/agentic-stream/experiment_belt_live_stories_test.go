package main

import (
	"maps"
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/testsupport/campaign"
)

var (
	liveFaults = map[string]string{"bearing": "conveyor_drive__lube_loss", "gearbox_seizing": "conveyor_drive__gear_oil_loss",
		"drive_tripped": "conveyor_drive__vfd_fault"}
	liveStories = map[string]liveStory{
		"s1-a":         savingTheLine("a", twinStories["s1-a"]),
		"s1-b":         savingTheLine("b", twinStories["s1-b"]),
		"s2-b":         {plant: "b", planned: lostDriveLimp, judge: lostDriveLimp.assertTheSavedLine},
		"s3-a":         savingTheLine("a", twinStories["s3-a"]),
		"s5-a":         savingTheLine("a", twinStories["s5-a"]),
		"s5b-a":        savingTheLine("a", twinStories["s5b-a"]),
		"s6-a-promote": masterLossTwins["s6-a-promote"].live("s6-promote"),
		"s6-a-stop":    masterLossTwins["s6-a-stop"].live("s6-stop"),
	}
)

type liveStory struct {
	plant   string
	barrier bool
	planned twinStory
	judge   func(t *testing.T, played twinOutcome)
}

func savingTheLine(plant string, planned twinStory) liveStory {
	return liveStory{plant: plant, barrier: true, planned: planned, judge: planned.assertTheSavedLine}
}

func (m masterLossTwin) live(plant string) liveStory {
	return liveStory{plant: plant, barrier: true, planned: m.planned(), judge: m.assertTheOutcome}
}

func (s liveStory) inTable(t *testing.T, table storyTable, name string) tableStory {
	t.Helper()
	played := table.story(t, name)
	if played.Plant != s.plant {
		t.Fatalf("the live story %s runs on plant %s, the story table's %s on plant %s", name, s.plant, name, played.Plant)
	}
	return played
}

func (s liveStory) subjects(t *testing.T, c conveyorTwin) campaign.Subjects {
	t.Helper()
	faults := maps.Clone(liveFaults)
	maps.Copy(faults, s.planned.faults())
	return s.planned.subjects(c.subjects(t, s.plant, s.planned.faulted, faults))
}

func (s liveStory) assertPlayed(t *testing.T, played twinOutcome, onset time.Time, limp string) {
	t.Helper()
	s.judge(t, played)
	assertRoutes(t, played.db, s.planned.routes)
	s.planned.assertReadBacks(t, played.db, limp)
	if !s.barrier {
		assertLimpFeedInTime(t, played.db, onset)
		return
	}
	assertR2PassedOnSettledFeatures(t, played.db, s.planned.settledSteps())
}

func (s twinStory) assertTheSavedLine(t *testing.T, played twinOutcome) {
	t.Helper()
	assertTheOutcomeSavesTheLine(t, played.grade, played.output, s.unharmed)
	s.judge(t, played.grade, played.ran)
	s.lost.assertReportedBeforeTheCause(t, played.runDir, s.fault)
}
