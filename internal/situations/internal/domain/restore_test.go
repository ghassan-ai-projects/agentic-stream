package domain_test

import (
	"bytes"
	"testing"
	"time"

	domain "github.com/ghassan-ai-projects/agentic-stream/internal/situations/internal/domain"
)

func TestRestoreRequiresTheSituationsIdentity(t *testing.T) {
	t.Parallel()
	complete := domain.Situation{SituationID: "sit-1", EntityType: "motor", EntityID: "motor-17"}
	tests := []struct {
		name   string
		mutate func(*domain.Situation)
	}{
		{"situation id", func(s *domain.Situation) { s.SituationID = "" }},
		{"entity type", func(s *domain.Situation) { s.EntityType = "" }},
		{"entity id", func(s *domain.Situation) { s.EntityID = "" }},
	}
	for _, tc := range tests {
		t.Run("missing "+tc.name, func(t *testing.T) {
			t.Parallel()
			s := complete
			tc.mutate(&s)
			requireErrorContaining(t, newEngine(t, vibrationSpec()).Restore(s), "restore situation requires identity")
		})
	}
	if err := newEngine(t, vibrationSpec()).Restore(complete); err != nil {
		t.Fatalf("Restore of a complete identity: %v", err)
	}
}

func TestRestoredSituationContinuesFromItsPersistedVersion(t *testing.T) {
	t.Parallel()
	engine := newEngine(t, vibrationSpec())
	restored := domain.Situation{
		SituationID: "sit-1", EntityType: "motor", EntityID: "motor-17", OccurrenceID: "occ-1",
		Type: "bearing_degradation", TenantID: "default", Version: 3, Phase: "watch", Severity: 30, Confidence: 1, Completeness: "provisional",
	}
	if err := engine.Restore(restored); err != nil {
		t.Fatal(err)
	}
	apply(t, engine, vibration(6.0, base))
	version := apply(t, engine, vibration(6.0, base.Add(2*time.Minute)))
	if len(version) != 1 || version[0].Version != 4 || version[0].PreviousVersion != 3 || version[0].Phase != "warning" || version[0].SituationID != "sit-1" {
		t.Fatalf("versions = %+v, want version 4 in warning continuing sit-1", version)
	}
}

func TestCurrentStateIsACopyWithItsCanonicalStateAndDigest(t *testing.T) {
	t.Parallel()
	engine := newEngine(t, vibrationSpec())
	published := apply(t, engine, vibration(5.0, base, "evt-1"))[0]
	state, blob, digest, found, err := engine.CurrentState(0, "motor", "motor-17")
	if err != nil || !found {
		t.Fatalf("CurrentState found=%v err=%v", found, err)
	}
	if !bytes.Equal(blob, published.StateJSON) || digest != published.StateSHA256 {
		t.Fatalf("state blob/digest differ from the published version: %s %s vs %s %s", blob, digest, published.StateJSON, published.StateSHA256)
	}
	state.Facts["facts.vibration_rms"] = 0.0
	state.Evidence[0] = "tampered"
	state.ConditionStart["x"] = base
	_, again, againDigest, _, _ := engine.CurrentState(0, "motor", "motor-17")
	if !bytes.Equal(again, blob) || againDigest != digest {
		t.Fatalf("editing the returned copy changed the engine's state: %s", again)
	}
}

func TestCurrentStateOfAnUnknownEntityIsNotFound(t *testing.T) {
	t.Parallel()
	engine := newEngine(t, vibrationSpec())
	apply(t, engine, vibration(5.0, base))
	for name, key := range map[string][3]any{
		"another partition":   {1, "motor", "motor-17"},
		"another entity type": {0, "pump", "motor-17"},
		"another entity":      {0, "motor", "motor-18"},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			_, blob, digest, found, err := engine.CurrentState(key[0].(int), key[1].(string), key[2].(string))
			if found || blob != nil || digest != "" || err != nil {
				t.Fatalf("CurrentState = %s %q found=%v err=%v, want not found", blob, digest, found, err)
			}
		})
	}
}

func TestResetForgetsEverySituation(t *testing.T) {
	t.Parallel()
	engine := newEngine(t, vibrationSpec())
	apply(t, engine, vibration(5.0, base))
	engine.Reset()
	if _, _, _, found, _ := engine.CurrentState(0, "motor", "motor-17"); found {
		t.Fatal("a situation survived Reset")
	}
	if versions := apply(t, engine, vibration(5.0, base.Add(time.Minute))); len(versions) != 1 || versions[0].Version != 1 {
		t.Fatalf("after Reset the next feature = %+v, want a fresh version 1", versions)
	}
}

func TestSituationThatViolatesTheSnapshotContractIsNeverPublished(t *testing.T) {
	t.Parallel()
	engine := newEngine(t, vibrationSpec())
	incomplete := domain.Situation{SituationID: "sit-1", EntityType: "motor", EntityID: "motor-17", Version: 3, Phase: "watch"}
	if err := engine.Restore(incomplete); err != nil {
		t.Fatal(err)
	}
	_, err := engine.ApplyFeature(t.Context(), vibration(2.0, base), base)
	requireErrorContaining(t, err, "validate snapshot")
}
