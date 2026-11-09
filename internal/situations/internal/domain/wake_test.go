package domain_test

import (
	"testing"
	"time"
)

func TestAWokenSituationPublishesItsCurrentFactsAsANewVersionInTheSamePhase(t *testing.T) {
	t.Parallel()
	engine := newEngine(t, vibrationSpec())
	opened := apply(t, engine, vibration(4.8, base))
	if len(opened) != 1 || opened[0].Phase != "watch" {
		t.Fatalf("opening versions = %+v, want one in watch", opened)
	}
	if moved := apply(t, engine, vibration(5.0, base.Add(time.Minute))); len(moved) != 0 {
		t.Fatalf("a fact change without a phase change published %+v, want nothing", moved)
	}
	woken, found, err := engine.Woken(opened[0].SituationID, base.Add(time.Minute))
	if err != nil || !found {
		t.Fatalf("Woken = %v, %v", found, err)
	}
	if woken.Version != opened[0].Version+1 || woken.Phase != "watch" || !woken.WatchFired || woken.Facts["facts.vibration_rms"] != 5.0 {
		t.Fatalf("woken version = v%d %s watch_fired %v facts %v, want v%d watch, woken, the current 5.0",
			woken.Version, woken.Phase, woken.WatchFired, woken.Facts, opened[0].Version+1)
	}
	if partition, held := engine.PartitionOf(opened[0].SituationID); !held || partition != 0 {
		t.Fatalf("PartitionOf = %d, %v; want partition 0", partition, held)
	}
}

func TestOnlyAnOpenSituationCanBeWoken(t *testing.T) {
	t.Parallel()
	engine := newEngine(t, vibrationSpec())
	apply(t, engine, vibration(4.0, base))
	if _, found, err := engine.Woken("sit_unknown", base); err != nil || found {
		t.Fatalf("Woken of an unknown situation = %v, %v; want not found", found, err)
	}
	if _, held := engine.PartitionOf("sit_unknown"); held {
		t.Fatal("PartitionOf an unknown situation reports it held")
	}
}
