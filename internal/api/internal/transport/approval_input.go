package transport

import (
	"encoding/json"
	"net/http"

	"github.com/ghassan-ai-projects/agentic-stream/internal/api/internal/domain"
)

func decodeApproval(w http.ResponseWriter, r *http.Request) (domain.ApprovalInput, error) {
	return domain.DecodeApprovalInput(http.MaxBytesReader(w, r.Body, 16<<10)) //nolint:wrapcheck // The domain rule's message is the operator-facing text.
}

func writeJSONProblem(w http.ResponseWriter, status int, problem domain.Problem) {
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(problem)
}
