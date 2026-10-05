package api

import (
	"crypto/ed25519"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

type approvalInput struct {
	Approver  string `json:"approver_id"`
	Approved  *bool  `json:"approved"`
	Signature []byte `json:"signature"`
	Reason    string `json:"reason"`
}

func decodeApproval(w http.ResponseWriter, r *http.Request) (approvalInput, error) {
	r.Body = http.MaxBytesReader(w, r.Body, 16<<10)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	var input approvalInput
	if err := decoder.Decode(&input); err != nil {
		return input, fmt.Errorf("decode approval: %w", err)
	}
	if err := completeApprovalInput(decoder, input); err != nil {
		return input, err
	}
	return input, nil
}

func completeApprovalInput(decoder *json.Decoder, input approvalInput) error {
	if input.Approved == nil || input.Approver == "" || len(input.Signature) != ed25519.SignatureSize || strings.TrimSpace(input.Reason) == "" {
		return fmt.Errorf("approver, decision, signature and reason are required")
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return fmt.Errorf("exactly one JSON document is required")
	}
	return nil
}

func writeJSONProblem(w http.ResponseWriter, status int, problem Problem) {
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(problem)
}
