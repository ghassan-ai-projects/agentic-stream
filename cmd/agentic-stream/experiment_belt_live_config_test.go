package main

import (
	"encoding/json"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/spec"
)

func TestEveryLiveStoryHasACompleteLiveConfiguration(t *testing.T) {
	t.Parallel()
	lease, err := time.ParseDuration(conveyorRouteLease)
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range slices.Sorted(maps.Keys(liveStories)) {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			story := liveStories[name]
			story.assertTheSpecPlansIt(t, compileConveyorSpec(t, editedSpec(t, story.planned.specPath(), t.TempDir(), liveBudget)))
			story.assertThePlantCarriesIt(t, lease)
		})
	}
}

func (s liveStory) assertTheSpecPlansIt(t *testing.T, compiled *spec.CompiledSpec) {
	t.Helper()
	assertAllIn(t, "the spec's intents", intentTypes(compiled), s.planned.routes...)
	assertAllIn(t, "the spec's diagnosis codes", diagnosisCodes(t, compiled), slices.Collect(maps.Keys(s.planned.faults()))...)
	if s.planned.prognosis != "" && !slices.ContainsFunc(compiled.Operators, func(op spec.Operator) bool { return op.Output == s.planned.prognosis }) {
		t.Errorf("no operator of the spec outputs the prognosis %s", s.planned.prognosis)
	}
}

func intentTypes(compiled *spec.CompiledSpec) []string {
	intents := make([]string, len(compiled.Actions.Intents))
	for i, intent := range compiled.Actions.Intents {
		intents[i] = intent.Type
	}
	return intents
}

func diagnosisCodes(t *testing.T, compiled *spec.CompiledSpec) []string {
	t.Helper()
	var catalog []struct {
		Code string `json:"code"`
	}
	if err := json.Unmarshal([]byte(compiled.Cognition.Executor.DiagnosisCatalog), &catalog); err != nil {
		t.Fatalf("decode the diagnosis catalog: %v", err)
	}
	codes := make([]string, len(catalog))
	for i, entry := range catalog {
		codes[i] = entry.Code
	}
	return codes
}

func (s liveStory) assertThePlantCarriesIt(t *testing.T, lease time.Duration) {
	t.Helper()
	catalog, err := plantWireCatalog(effectProfileOptions{DeviceCatalog: conveyorCatalogOf(s.plant), DispatchLease: lease})
	if err != nil {
		t.Fatalf("plant %s: %v", s.plant, err)
	}
	routes, kinds := readConveyorRoutes(t, s.plant), siteKinds(t, conveyorSiteOf(s.plant))
	devices, operations := slices.Collect(maps.Keys(kinds)), []string{}
	for _, route := range s.planned.routes {
		if !catalog.NamesRoute(route) {
			t.Errorf("plant %s's catalog has no route %s", s.plant, route)
		}
		assertAllIn(t, "plant "+s.plant+"'s site devices", devices, slices.Collect(maps.Values(routes[route].TargetBindings))...)
		operations = append(operations, routes[route].Operation)
	}
	for _, readBack := range s.planned.readBacks {
		assertAllIn(t, "the operations of the planned routes", operations, readBack[0])
	}
	assertAllIn(t, "plant "+s.plant+"'s site devices", devices, slices.DeleteFunc([]string{s.planned.faulted, s.planned.device}, func(d string) bool { return d == "" })...)
	for _, kind := range kinds {
		if _, err := os.Stat(filepath.Join(conveyorKinds, kind+".kind.json")); err != nil {
			t.Errorf("plant %s's site names the kind %s: %v", s.plant, kind, err)
		}
	}
}

func assertAllIn(t *testing.T, of string, have []string, want ...string) {
	t.Helper()
	for _, value := range want {
		if !slices.Contains(have, value) {
			t.Errorf("%s %v hold no %s", of, have, value)
		}
	}
}

func TestTheLiveWiringCarriesTheS6PromoteStepThroughTheMeteredWorker(t *testing.T) {
	t.Parallel()
	story, out := liveStories["s6-a-promote"], t.TempDir()
	if story.plant != promotedFollower.plant {
		t.Fatalf("the live story runs on plant %s, the promoted follower's line on %s", story.plant, promotedFollower.plant)
	}
	upstream := filepath.Join(privateSocketDir(t), "reasoner.sock")
	serveWorker(t, upstream, answerTheMastersTrip(masterLossSteps))
	meter, workerSocket := serveMeteredTamoz(t, out, upstream)
	promotedFollower.assertTheLineTakesItsStepOn(t, startLiveServe(t, editedSpec(t, story.planned.specPath(), out, liveBudget), story.plant, out, workerSocket))
	if meter.episodes.Load() == 0 {
		t.Fatal("no episode passed the metered worker")
	}
}
