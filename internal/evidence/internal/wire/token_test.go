package wire

import (
	"encoding/base64"
	"strings"
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/evidence/internal/domain"
)

func TestTokenEncodingAndIntegrity(t *testing.T) {
	now := time.Date(2026, 8, 12, 12, 0, 0, 0, time.UTC)
	s := domain.Scope{KeyID: "k1", Issuer: "runtime", Audience: "tools", TokenID: "fixed", EpisodeID: "e", AttemptID: "a", Fence: 1, TenantID: "t", SituationID: "s", SituationVersion: 1, EntityID: "x", Tools: []string{"evidence.get"}, IssuedAt: now, NotBefore: now, ExpiresAt: now.Add(time.Minute), From: now, Until: now, MaxRows: 1, MaxBytes: 100, Traceparent: "trace", RuntimeEpoch: "epoch"}
	key := []byte("01234567890123456789012345678901")
	token, err := SignToken(s, key)
	if err != nil {
		t.Fatal(err)
	}
	id, raw, err := SignedPayload(token, map[string][]byte{"k1": key})
	if err != nil {
		t.Fatal(err)
	}
	const want = `{"iss":"runtime","aud":"tools","episode_id":"e","attempt_id":"a","fence":1,"tenant_id":"t","situation_id":"s","situation_version":1,"entity_id":"x","jti":"fixed","iat":"2026-08-12T12:00:00.000000000Z","tools":["evidence.get"],"nbf":"2026-08-12T12:00:00.000000000Z","exp":"2026-08-12T12:01:00.000000000Z","max_rows":1,"max_bytes":100,"from":"2026-08-12T12:00:00.000000000Z","until":"2026-08-12T12:00:00.000000000Z","traceparent":"trace","runtime_epoch":"epoch"}`
	if string(raw) != want {
		t.Fatalf("claims changed: %s", raw)
	}
	got, err := DecodeScope(raw, id)
	if err != nil || got.TokenID != s.TokenID || !got.ExpiresAt.Equal(s.ExpiresAt) {
		t.Fatalf("decoded=%+v err=%v", got, err)
	}
	bad := append([]byte(nil), token...)
	bad[len(bad)-1] ^= 1
	for name, value := range map[string][]byte{"format": []byte("v2.bad"), "tamper": bad, "signature": []byte(strings.Join([]string{"v1", "k1", base64.RawURLEncoding.EncodeToString(raw), "?"}, "."))} {
		t.Run(name, func(t *testing.T) {
			if _, _, err := SignedPayload(value, map[string][]byte{"k1": key}); err == nil {
				t.Fatal("invalid token accepted")
			}
		})
	}
	if _, _, err := SignedPayload(token, nil); err == nil {
		t.Fatal("unknown key accepted")
	}
	if _, err := DecodeScope([]byte(`{}`), "k1"); err == nil {
		t.Fatal("missing times accepted")
	}
	if _, err := DecodeScope([]byte(`garbage`), "k1"); err == nil {
		t.Fatal("invalid JSON accepted")
	}
}

func TestOpaqueIdentities(t *testing.T) {
	if got, err := TokenID("fixed"); err != nil || got != "fixed" {
		t.Fatalf("existing=%s err=%v", got, err)
	}
	a, err := TokenID("")
	if err != nil {
		t.Fatal(err)
	}
	b, err := NewRuntimeEpoch()
	if err != nil || a == b || len(a) != 22 || len(b) != 22 {
		t.Fatalf("identities=%q/%q err=%v", a, b, err)
	}
}
