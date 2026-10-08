package wire

import (
	"crypto/sha256"
	"encoding/hex"
	"testing"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/ghassan-ai-projects/agentic-stream/internal/contractsv1"
	"github.com/ghassan-ai-projects/agentic-stream/internal/evidence/internal/domain"
	runtimev1 "github.com/ghassan-ai-projects/agentic-stream/proto/agenticstream/runtime/v1"
)

func ledgerTestCall() domain.Call {
	return domain.Call{EpisodeID: "episode-1", CallID: "call-1", ToolName: "evidence.get", TenantID: "tenant-1", SituationID: "situation-1", SituationVersion: 1, EntityID: "motor-1", Arguments: domain.EvidenceGetArguments{EntityID: "motor-1"}, Deadline: time.Date(2026, 8, 12, 12, 1, 0, 0, time.UTC), AttemptID: "attempt-1", Fence: 1, Trace: traceForLedger(), MaxRows: 1, MaxBytes: 100, From: time.Date(2026, 8, 12, 11, 0, 0, 0, time.UTC), Until: time.Date(2026, 8, 12, 12, 0, 0, 0, time.UTC)}
}
func traceForLedger() contractsv1.TraceContext {
	return contractsv1.TraceContext{Traceparent: "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01"}
}
func TestEvidenceFingerprintEncodingIsStable(t *testing.T) {
	const document = `{"episode_id":"episode-1","call_id":"call-1","tool_name":"evidence.get","tenant_id":"tenant-1","situation_id":"situation-1","entity_id":"motor-1","attempt_id":"attempt-1","situation_version":1,"fence":1,"arguments":{"entity_id":"motor-1"},"deadline":"2026-08-12T12:01:00.000000000Z","from":"2026-08-12T11:00:00.000000000Z","until":"2026-08-12T12:00:00.000000000Z","max_rows":1,"max_bytes":100,"traceparent":"00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01","tracestate":""}`
	want := sha256.Sum256([]byte(document))
	got, err := CallFingerprint(ledgerTestCall())
	if err != nil || hex.EncodeToString(got) != hex.EncodeToString(want[:]) {
		t.Fatalf("fingerprint = %x, err = %v, want %x", got, err, want)
	}
}
func TestClosedArgumentsAndEnvelopeCodec(t *testing.T) {
	for _, raw := range []string{`{}`, `{"entity_id":1}`, `{"entity_id":"x","extra":true}`, `{"entity_id":"x"}{}`} {
		if _, err := DecodeEvidenceGetArguments([]byte(raw)); err == nil {
			t.Fatalf("arguments=%s", raw)
		}
	}
	args, err := DecodeEvidenceGetArguments([]byte(`{"entity_id":"x"}`))
	if err != nil || args.EntityID != "x" {
		t.Fatal(args, err)
	}
	req := &runtimev1.EvidenceToolCall{EpisodeId: "e", CallId: "c", AttemptId: "a", Fence: 1, Traceparent: "trace", TimeFrom: timestamppb.New(time.Unix(10, 0)), TimeUntil: timestamppb.New(time.Unix(20, 0)), Deadline: &timestamppb.Timestamp{Nanos: -1}}
	got := DecodeEnvelope(req)
	if !got.From.Valid || !got.From.Present || got.Deadline.Valid || got.EncodedSize == 0 {
		t.Fatalf("envelope=%+v", got)
	}
	result := ResultMessage(req, domain.QueryResult{JSON: []byte(`[]`), RowCount: 2})
	if result.GetEpisodeId() != "e" || result.GetRowCount() != 2 || result.GetResultBytes() != 2 || len(result.GetResultSha256()) != 32 {
		t.Fatal(result)
	}
}
