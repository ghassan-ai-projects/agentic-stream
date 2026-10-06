package domain

import (
	"context"
	"errors"
	"fmt"
	"testing"
)

type stubLedger struct{}

func (stubLedger) Entries(context.Context) ([]RecordedEntry, error) { return nil, nil }

type stubBaseline struct{}

func (stubBaseline) ExecuteBaseline(context.Context, ShadowInput) (ShadowOutput, error) {
	return ShadowOutput{}, nil
}

type stubShadow struct{}

func (stubShadow) ExecuteShadow(context.Context, ShadowInput) (ShadowOutput, error) {
	return ShadowOutput{}, nil
}

type stubSimulator struct{}

func (stubSimulator) Simulate(context.Context, SimulatedCommand) (map[string]any, error) {
	return nil, nil
}

func TestCapabilitiesValidateFailsClosedPerMode(t *testing.T) {
	t.Parallel()
	complete := Capabilities{RecordedLedger: stubLedger{}, BaselineExecutor: stubBaseline{}, ShadowExecutor: stubShadow{}, Simulator: stubSimulator{}}
	for _, tc := range []struct {
		mode Mode
		caps Capabilities
		want error
	}{
		{ModeRecorded, complete, nil},
		{ModeRecorded, Capabilities{}, ErrModeCapabilityRequired},
		{ModeShadow, complete, nil},
		{ModeShadow, Capabilities{ShadowExecutor: stubShadow{}}, ErrModeCapabilityRequired},
		{ModeShadow, Capabilities{BaselineExecutor: stubBaseline{}}, ErrModeCapabilityRequired},
		{ModeCounterfactual, Capabilities{Simulator: stubSimulator{}}, nil},
		{ModeCounterfactual, Capabilities{}, ErrModeCapabilityRequired},
		{Mode("future"), complete, ErrUnsupportedMode},
	} {
		t.Run(string(tc.mode)+fmt.Sprintf("%T", tc.caps.RecordedLedger), func(t *testing.T) {
			t.Parallel()
			err := tc.caps.Validate(tc.mode)
			if tc.want == nil {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			if !errors.Is(err, tc.want) {
				t.Fatalf("error = %v want %v", err, tc.want)
			}
		})
	}
}

func TestAdmitCapabilitiesAcceptsAtMostOneSet(t *testing.T) {
	t.Parallel()
	single := Capabilities{Simulator: stubSimulator{}}
	got, err := AdmitCapabilities(nil)
	if err != nil || got.RecordedLedger != nil || got.Simulator != nil {
		t.Fatalf("empty admission = %+v err=%v", got, err)
	}
	got, err = AdmitCapabilities([]Capabilities{single})
	if err != nil || got.Simulator == nil {
		t.Fatalf("single admission = %+v err=%v", got, err)
	}
	if _, err = AdmitCapabilities([]Capabilities{single, single}); err == nil {
		t.Fatal("duplicate capability sets were accepted")
	}
}

func TestWorkerAwareModeClassification(t *testing.T) {
	t.Parallel()
	for mode, want := range map[Mode]bool{
		ModeDeterministic: false, ModeRecorded: true, ModeShadow: true, ModeCounterfactual: true,
	} {
		if got := WorkerAwareMode(mode); got != want {
			t.Fatalf("WorkerAwareMode(%s) = %v want %v", mode, got, want)
		}
	}
}

func TestAdmitSimulatedCommandRejectsIncompleteAndDuplicate(t *testing.T) {
	t.Parallel()
	seen := map[string]struct{}{}
	complete := SimulatedCommand{CommandID: "one", Route: "simulated", Target: "motor"}
	if err := AdmitSimulatedCommand(complete, seen); err != nil {
		t.Fatalf("complete command rejected: %v", err)
	}
	if err := AdmitSimulatedCommand(complete, seen); err == nil {
		t.Fatal("duplicate command accepted")
	}
	for name, command := range map[string]SimulatedCommand{
		"no id":     {Route: "simulated", Target: "motor"},
		"no route":  {CommandID: "two", Target: "motor"},
		"no target": {CommandID: "three", Route: "simulated"},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if err := AdmitSimulatedCommand(command, map[string]struct{}{}); err == nil {
				t.Fatal("incomplete command accepted")
			}
		})
	}
}

func TestAllHashesEqual(t *testing.T) {
	t.Parallel()
	if !AllHashesEqual(nil) || !AllHashesEqual([]Result{{VersionsHash: "a"}, {VersionsHash: "a"}}) {
		t.Fatal("equal hashes reported unequal")
	}
	if AllHashesEqual([]Result{{VersionsHash: "a"}, {VersionsHash: "b"}}) {
		t.Fatal("unequal hashes reported equal")
	}
}

func TestEpisodeKeyAndViews(t *testing.T) {
	t.Parallel()
	episode := ReplayEpisode{SituationID: "s1", SituationVersion: 2, TriggerID: "t1"}
	if key := EpisodeKey("s1", 2, "t1"); key != "s1/2/t1" {
		t.Fatalf("key = %q", key)
	}
	view := EpisodeViews([]ReplayEpisode{episode})[0]
	if view.EpisodeKey != "s1/2/t1" || view.SituationID != "s1" {
		t.Fatalf("view = %+v", view)
	}
}
