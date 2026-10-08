package domain

import (
	"reflect"
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/episodeledger"
)

func TestAdmissionRecordSurvivesTheRequestRoundTrip(t *testing.T) {
	t.Parallel()
	digest := func(seed byte) []byte { return append(make([]byte, 31), seed) }
	want := episodeledger.Admission{
		EpisodeID: "epi", SchedulerItemID: "item", TenantID: "tenant", SituationID: "sit", SituationVersion: 3,
		ExecutorName: "exec", ExecutorVersion: "v2", ModelPolicy: "policy", PromptVersion: "p1",
		SnapshotSHA256: digest(1), PromptSHA256: digest(2), ObjectiveSHA256: digest(3), AdmissionKey: digest(4),
		RequestJSON: []byte(`{"k":1}`), DispatchPolicy: "active", PolicyEpoch: "epoch",
	}
	persisted := reflect.ValueOf(want)
	for i := range persisted.NumField() {
		if name := persisted.Type().Field(i).Name; name != "Kind" && persisted.Field(i).IsZero() {
			t.Fatalf("fixture leaves admission field %s zero; the round trip would not cover it", name)
		}
	}
	req := AdmittedRequest(want)
	digests, err := DecodeRequestDigests(&req)
	if err != nil {
		t.Fatal(err)
	}
	if got := AdmittedEpisode(&req, digests); !reflect.DeepEqual(got, want) {
		t.Fatalf("round trip = %+v, want %+v", got, want)
	}
}
