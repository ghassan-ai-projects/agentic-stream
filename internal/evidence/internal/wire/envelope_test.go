package wire

import (
	"crypto/sha256"
	"reflect"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/ghassan-ai-projects/agentic-stream/internal/evidence/internal/domain"
	runtimev1 "github.com/ghassan-ai-projects/agentic-stream/proto/agenticstream/runtime/v1"
)

func TestDecodeEnvelopeCopiesEveryRequestFieldWithoutGrantingAuthority(t *testing.T) {
	t.Parallel()
	from, until, deadline := time.Unix(10, 0).UTC(), time.Unix(20, 0).UTC(), time.Unix(30, 0).UTC()
	request := &runtimev1.EvidenceToolCall{
		ProtocolVersion: "1.0", EpisodeId: "e", CallId: "c", ToolName: "evidence.get", TenantId: "t", SituationId: "s",
		EntityId: "x", AttemptId: "a", Fence: 2, SituationVersion: 3, MaxRows: 4, MaxBytes: 5,
		ArgumentsJson: []byte(`{"entity_id":"x"}`), CapabilityToken: []byte("token"),
		Deadline: timestamppb.New(deadline), TimeFrom: timestamppb.New(from), TimeUntil: timestamppb.New(until),
		Traceparent: "trace", Tracestate: "vendor=1",
	}
	got := DecodeEnvelope(request)
	want := domain.Envelope{
		ProtocolVersion: "1.0", EpisodeID: "e", CallID: "c", ToolName: "evidence.get", TenantID: "t", SituationID: "s",
		EntityID: "x", AttemptID: "a", Fence: 2, SituationVersion: 3, MaxRows: 4, MaxBytes: 5,
		ArgumentsJSON: []byte(`{"entity_id":"x"}`), CapabilityToken: []byte("token"),
		Deadline:    domain.Timestamp{Value: deadline, Present: true, Valid: true},
		From:        domain.Timestamp{Value: from, Present: true, Valid: true},
		Until:       domain.Timestamp{Value: until, Present: true, Valid: true},
		Traceparent: "trace", Tracestate: "vendor=1", EncodedSize: proto.Size(request),
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("envelope = %+v\nwant       %+v", got, want)
	}
}

func TestDecodeEnvelopeKeepsTimestampPresenceAndValiditySeparate(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name        string
		stamp       *timestamppb.Timestamp
		wantPresent bool
		wantValid   bool
	}{
		{"absent", nil, false, false},
		{"valid", timestamppb.New(time.Unix(10, 0)), true, true},
		{"negative nanoseconds", &timestamppb.Timestamp{Nanos: -1}, true, false},
		{"seconds beyond the supported range", &timestamppb.Timestamp{Seconds: 1 << 40}, true, false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			got := DecodeEnvelope(&runtimev1.EvidenceToolCall{Deadline: test.stamp}).Deadline
			if got.Present != test.wantPresent || got.Valid != test.wantValid {
				t.Fatalf("deadline = present %v valid %v, want %v/%v", got.Present, got.Valid, test.wantPresent, test.wantValid)
			}
		})
	}
}

func TestResultMessageEchoesTheRequestIdentityAndHashesTheExactBytes(t *testing.T) {
	t.Parallel()
	request := &runtimev1.EvidenceToolCall{EpisodeId: "e", CallId: "c", AttemptId: "a", Fence: 7, Traceparent: "trace", Tracestate: "vendor=1"}
	result := domain.QueryResult{JSON: []byte(`{"rows":[1,2]}`), RowCount: 2}
	got := ResultMessage(request, result)
	hash := sha256.Sum256(result.JSON)
	if got.GetEpisodeId() != "e" || got.GetCallId() != "c" || got.GetAttemptId() != "a" || got.GetFence() != 7 ||
		got.GetTraceparent() != "trace" || got.GetTracestate() != "vendor=1" {
		t.Fatalf("echoed identity = %v", got)
	}
	if string(got.GetResultJson()) != string(result.JSON) || got.GetResultBytes() != uint64(len(result.JSON)) || got.GetRowCount() != 2 ||
		string(got.GetResultSha256()) != string(hash[:]) {
		t.Fatalf("result = %v", got)
	}
}
