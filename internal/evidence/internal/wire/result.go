package wire

import (
	"crypto/sha256"

	"github.com/ghassan-ai-projects/agentic-stream/internal/evidence/internal/domain"
	runtimev1 "github.com/ghassan-ai-projects/agentic-stream/proto/agenticstream/runtime/v1"
)

// ResultMessage preserves the response bytes, hash and echoed request identity.
func ResultMessage(req *runtimev1.EvidenceToolCall, result domain.QueryResult) *runtimev1.EvidenceToolResult {
	hash := sha256.Sum256(result.JSON)
	return resultMessageWithHash(req, result, hash)
}
func resultMessageWithHash(req *runtimev1.EvidenceToolCall, result domain.QueryResult, hash [sha256.Size]byte) *runtimev1.EvidenceToolResult {
	return &runtimev1.EvidenceToolResult{EpisodeId: req.GetEpisodeId(), CallId: req.GetCallId(), ResultJson: result.JSON, ResultSha256: hash[:], ResultBytes: uint64(len(result.JSON)), RowCount: result.RowCount, AttemptId: req.GetAttemptId(), Fence: req.GetFence(), Traceparent: req.GetTraceparent(), Tracestate: req.GetTracestate()}
}
