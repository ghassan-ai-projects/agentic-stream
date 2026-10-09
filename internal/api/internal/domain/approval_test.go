package domain

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

func TestBearerMatching(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name          string
		match         func(header, token string) bool
		header, token string
		want          bool
	}{
		{"exact: header equals Bearer plus token", ExactBearer, "Bearer tok", "tok", true},
		{"exact: extra space", ExactBearer, "Bearer  tok", "tok", false},
		{"exact: no scheme", ExactBearer, "tok", "tok", false},
		{"exact: token prefix only", ExactBearer, "Bearer to", "tok", false},
		{"loose: surrounding space is ignored", LooseBearer, "Bearer  tok ", " tok ", true},
		{"loose: other token", LooseBearer, "Bearer other", "tok", false},
		{"loose: an empty expected token never matches", LooseBearer, "Bearer ", "", false},
		{"loose: a blank expected token never matches", LooseBearer, "Bearer   ", "  ", false},
		{"loose: a missing header never matches a real token", LooseBearer, "", "tok", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := tt.match(tt.header, tt.token); got != tt.want {
				t.Fatalf("match(%q, %q) = %v, want %v", tt.header, tt.token, got, tt.want)
			}
		})
	}
}

func TestDecodeApprovalInputRequiresOneCompleteDocument(t *testing.T) {
	t.Parallel()
	signature := strings.Repeat("A", 86) + "=="
	valid := `{"approver_id":"a","approved":true,"signature":"` + signature + `","reason":"ok"}`
	tests := []struct {
		name, body string
		wantErr    string
	}{
		{"complete document", valid, ""},
		{"a denial is a decision too", strings.Replace(valid, `"approved":true`, `"approved":false`, 1), ""},
		{"unknown field", strings.TrimSuffix(valid, "}") + `,"x":1}`, "decode approval"},
		{"missing everything but the approver", `{"approver_id":"a"}`, "approver, decision, signature and reason are required"},
		{"missing decision", strings.Replace(valid, `"approved":true,`, "", 1), "approver, decision, signature and reason are required"},
		{"blank reason", strings.Replace(valid, `"reason":"ok"`, `"reason":"  "`, 1), "approver, decision, signature and reason are required"},
		{"short signature", strings.Replace(valid, signature, "AA==", 1), "approver, decision, signature and reason are required"},
		{"two documents", valid + valid, "exactly one JSON document is required"},
		{"trailing garbage", valid + "garbage", "exactly one JSON document is required"},
		{"not json", "nope", "decode approval"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_, err := DecodeApprovalInput(bytes.NewBufferString(tt.body))
			if tt.wantErr == "" && err != nil || tt.wantErr != "" && (err == nil || !strings.Contains(err.Error(), tt.wantErr)) {
				t.Fatalf("DecodeApprovalInput error = %v, want %q", err, tt.wantErr)
			}
		})
	}
}

func TestApprovalConfigIsConfiguredOnlyWithEveryCallbackAndCredential(t *testing.T) {
	t.Parallel()
	full := ApprovalConfig{
		Present:     func(context.Context, ApprovalSelection) (any, error) { return nil, nil },
		Resolve:     func(context.Context, ApprovalSubmission) (any, error) { return nil, nil },
		ErrorStatus: func(error) int { return 500 },
		Token:       "token", Relay: "relay",
	}
	if !full.Configured() {
		t.Fatal("complete config reported unconfigured")
	}
	for name, withoutPart := range map[string]func(*ApprovalConfig){
		"present":      func(c *ApprovalConfig) { c.Present = nil },
		"resolve":      func(c *ApprovalConfig) { c.Resolve = nil },
		"error status": func(c *ApprovalConfig) { c.ErrorStatus = nil },
		"token":        func(c *ApprovalConfig) { c.Token = "" },
		"relay":        func(c *ApprovalConfig) { c.Relay = "" },
	} {
		t.Run("without "+name, func(t *testing.T) {
			t.Parallel()
			cfg := full
			withoutPart(&cfg)
			if cfg.Configured() {
				t.Fatalf("config without %s reported configured", name)
			}
		})
	}
}
