package campaign

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
)

var errRelayRefused = errors.New("the approval endpoint refused the request")

// Relay is the approval relay: it fetches a request's signing bytes for its
// approver, signs them with the approver's key and submits the decision.
type Relay struct {
	BaseURL  string
	Token    string
	Approver string
	Key      ed25519.PrivateKey
	Client   *http.Client
}

// Presentation is what the endpoint presents for a decision: the bytes to sign
// and, for a rule proposal, the proposal with its replay report.
type Presentation struct {
	SigningBytes []byte `json:"signing_bytes"`
	Proposal     *struct {
		Replay ReplayReport `json:"replay"`
	} `json:"proposal"`
}

// Present fetches the request as the approver would decide it.
func (r Relay) Present(ctx context.Context, id string, approved bool) (Presentation, error) {
	query := url.Values{"approver": {r.Approver}, "approved": {strconv.FormatBool(approved)}}
	var presented Presentation
	err := r.call(ctx, http.MethodGet, id+"?"+query.Encode(), nil, &presented)
	return presented, err
}

// Resolve signs and submits the approver's decision.
func (r Relay) Resolve(ctx context.Context, id string, approved bool) error {
	presented, err := r.Present(ctx, id, approved)
	if err != nil {
		return err
	}
	body, err := json.Marshal(map[string]any{"approver_id": r.Approver, "approved": approved,
		"signature": ed25519.Sign(r.Key, presented.SigningBytes), "reason": "campaign approver"})
	if err != nil {
		return fmt.Errorf("encode the decision on %s: %w", id, err)
	}
	return r.call(ctx, http.MethodPost, id, bytes.NewReader(body), nil)
}

func (r Relay) call(ctx context.Context, method, path string, body io.Reader, into any) error {
	request, err := http.NewRequestWithContext(ctx, method, r.BaseURL+"/v1/approvals/"+path, body)
	if err != nil {
		return fmt.Errorf("build %s %s: %w", method, path, err)
	}
	request.Header.Set("Authorization", "Bearer "+r.Token)
	response, err := r.Client.Do(request)
	if err != nil {
		return fmt.Errorf("%s %s: %w", method, path, err)
	}
	defer func() { _ = response.Body.Close() }()
	return readResponse(response, method+" "+path, into)
}

func readResponse(response *http.Response, call string, into any) error {
	data, err := io.ReadAll(response.Body)
	if err != nil {
		return fmt.Errorf("read %s: %w", call, err)
	}
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("%w: %s: %d %s", errRelayRefused, call, response.StatusCode, data)
	}
	return decodeInto(data, into)
}

func decodeInto(data []byte, into any) error {
	if into == nil {
		return nil
	}
	if err := json.Unmarshal(data, into); err != nil {
		return fmt.Errorf("decode the approval response: %w", err)
	}
	return nil
}
