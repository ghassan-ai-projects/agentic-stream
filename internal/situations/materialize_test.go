package situations

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/spec"
)

func TestMaterializationKeepsCanonicalEvidenceAndPrivateState(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	engine := &Engine{spec: &spec.CompiledSpec{Digest: "sha256:" + strings.Repeat("0", 64)}, tenantID: "tenant"}
	sit := &Situation{
		SituationID: "s1", TenantID: "tenant", Type: "test", Version: 2, Phase: "watch", PreviousPhase: "candidate",
		EntityType: "motor", EntityID: "m1", Completeness: "on_time", Confidence: 1,
		LatestEventTime: now, Facts: map[string]any{"level": 3.0, "level_event_time": now.Format(time.RFC3339Nano)},
		Evidence: map[string]struct{}{"evt-z": {}, "evt-a": {}}, ConditionStart: map[string]time.Time{"watch": now},
	}
	first, err := engine.materialize(sit, now)
	if err != nil {
		t.Fatal(err)
	}
	if len(first.Evidence) != 2 || first.Evidence[0] != "evt-a" || first.Evidence[1] != "evt-z" || first.PreviousVersion != 1 {
		t.Fatalf("published version=%+v", first)
	}
	if _, exists := first.Facts["level_event_time"]; exists || !bytes.Contains(first.StateJSON, []byte("level_event_time")) {
		t.Fatal("internal event time must be persisted but excluded from published facts")
	}
	sit.Evidence = map[string]struct{}{"evt-a": {}, "evt-z": {}}
	second, err := engine.materialize(sit, now)
	if err != nil || !bytes.Equal(first.SnapshotJSON, second.SnapshotJSON) || first.SnapshotSHA256 != second.SnapshotSHA256 || first.StateSHA256 != second.StateSHA256 {
		t.Fatalf("map insertion order changed materialization: err=%v", err)
	}
	sit.Facts["level"] = 8.0
	sit.ConditionStart["watch"] = now.Add(time.Hour)
	if first.Facts["level"] != 3.0 || !first.ConditionStart["watch"].Equal(now) {
		t.Fatal("later state mutation changed a published version")
	}
}
