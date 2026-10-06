package domain

import (
	"crypto/ed25519"
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

// ApprovalInput is the JSON body of an approval decision.
type ApprovalInput struct {
	Approver  string `json:"approver_id"`
	Approved  *bool  `json:"approved"`
	Signature []byte `json:"signature"`
	Reason    string `json:"reason"`
}

// DecodeApprovalInput reads exactly one approval document with no unknown
// fields and requires the approver, decision, signature and reason.
func DecodeApprovalInput(body io.Reader) (ApprovalInput, error) {
	decoder := json.NewDecoder(body)
	decoder.DisallowUnknownFields()
	var input ApprovalInput
	if err := decoder.Decode(&input); err != nil {
		return input, fmt.Errorf("decode approval: %w", err)
	}
	if err := completeApprovalInput(decoder, input); err != nil {
		return input, err
	}
	return input, nil
}

func completeApprovalInput(decoder *json.Decoder, input ApprovalInput) error {
	if input.Approved == nil || input.Approver == "" || len(input.Signature) != ed25519.SignatureSize || strings.TrimSpace(input.Reason) == "" {
		return fmt.Errorf("approver, decision, signature and reason are required")
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return fmt.Errorf("exactly one JSON document is required")
	}
	return nil
}

// ExactBearer reports whether header is exactly "Bearer " plus token, in
// constant time.
func ExactBearer(header, token string) bool {
	return subtle.ConstantTimeCompare([]byte(header), []byte("Bearer "+token)) == 1
}

// LooseBearer reports whether header carries expected as a bearer token,
// ignoring surrounding space, in constant time. An empty expected never matches.
func LooseBearer(header, expected string) bool {
	expected = strings.TrimSpace(expected)
	provided := strings.TrimSpace(strings.TrimPrefix(header, "Bearer "))
	return expected != "" && subtle.ConstantTimeCompare([]byte(provided), []byte(expected)) == 1
}
