package wire

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"reflect"
	"strings"
	"testing"
)

func TestSignedTokenRoundTripsEveryClaim(t *testing.T) {
	t.Parallel()
	want := signedScope()
	token, err := SignToken(want, signingKey)
	if err != nil {
		t.Fatalf("SignToken: %v", err)
	}
	keyID, claims, err := SignedPayload(token, map[string][]byte{"k1": signingKey})
	if err != nil {
		t.Fatalf("SignedPayload: %v", err)
	}
	got, err := DecodeScope(claims, keyID)
	if err != nil {
		t.Fatalf("DecodeScope: %v", err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("decoded scope = %+v, want %+v", got, want)
	}
}

func TestSignedTokenClaimsAreTheV1Contract(t *testing.T) {
	t.Parallel()
	scope := signedScope()
	scope.Tracestate = ""
	token, err := SignToken(scope, signingKey)
	if err != nil {
		t.Fatalf("SignToken: %v", err)
	}
	_, claims, err := SignedPayload(token, map[string][]byte{"k1": signingKey})
	if err != nil {
		t.Fatalf("SignedPayload: %v", err)
	}
	const want = `{"iss":"runtime","aud":"tools","episode_id":"e","attempt_id":"a","fence":1,"tenant_id":"t","situation_id":"s","situation_version":1,"entity_id":"x","jti":"fixed","iat":"2026-08-12T12:00:00.000000000Z","tools":["evidence.get"],"nbf":"2026-08-12T12:00:00.000000000Z","exp":"2026-08-12T12:01:00.000000000Z","max_rows":1,"max_bytes":100,"from":"2026-08-12T11:00:00.000000000Z","until":"2026-08-12T12:00:00.000000000Z","traceparent":"trace","runtime_epoch":"epoch"}`
	if string(claims) != want {
		t.Fatalf("claims = %s\nwant    %s", claims, want)
	}
	if parts := strings.Split(string(token), "."); len(parts) != 4 || parts[0] != "v1" || parts[1] != "k1" {
		t.Fatalf("token framing = %q, want v1.<key>.<claims>.<signature>", token)
	}
}

func TestSignedPayloadRefusesMalformedAndTamperedTokens(t *testing.T) {
	t.Parallel()
	token, err := SignToken(signedScope(), signingKey)
	if err != nil {
		t.Fatalf("SignToken: %v", err)
	}
	parts := strings.Split(string(token), ".")
	forgedClaims := base64.RawURLEncoding.EncodeToString([]byte(`{"tenant_id":"another"}`))
	flipped := append([]byte(nil), token...)
	flipped[len(flipped)-10] = 'A'
	if token[len(token)-10] == 'A' {
		flipped[len(flipped)-10] = 'B'
	}
	keys := map[string][]byte{"k1": signingKey}
	tests := []struct {
		name  string
		token []byte
		keys  map[string][]byte
		want  string
	}{
		{"empty token", nil, keys, "format"},
		{"unknown version", []byte("v2." + strings.Join(parts[1:], ".")), keys, "format"},
		{"too few parts", []byte("v1.k1.payload"), keys, "format"},
		{"too many parts", append(token, []byte(".extra")...), keys, "format"},
		{"empty key identity", []byte("v1.." + parts[2] + "." + parts[3]), keys, "format"},
		{"unknown key", token, map[string][]byte{"other": signingKey}, "unknown capability key"},
		{"no keys", token, nil, "unknown capability key"},
		{"key below the minimum length", token, map[string][]byte{"k1": signingKey[:16]}, "unknown capability key"},
		{"signed with another key", token, map[string][]byte{"k1": []byte("abcdefghijklmnopqrstuvwxyz012345")}, "signature"},
		{"altered signature character", flipped, keys, "signature"},
		{"claims replaced under the old signature", []byte(strings.Join([]string{"v1", "k1", forgedClaims, parts[3]}, ".")), keys, "signature"},
		{"signature not base64", []byte(strings.Join([]string{"v1", "k1", parts[2], "?"}, ".")), keys, "signature"},
		{"truncated signature", []byte(strings.Join([]string{"v1", "k1", parts[2], parts[3][:10]}, ".")), keys, "signature"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			_, _, err := SignedPayload(test.token, test.keys)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("SignedPayload error = %v, want one containing %q", err, test.want)
			}
		})
	}
}

func TestSignedPayloadRefusesCorrectlySignedClaimsThatAreNotBase64(t *testing.T) {
	t.Parallel()
	signed := "v1.k1.?"
	mac := hmac.New(sha256.New, signingKey)
	_, _ = mac.Write([]byte(signed))
	token := []byte(signed + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil)))
	_, _, err := SignedPayload(token, map[string][]byte{"k1": signingKey})
	if err == nil || !strings.Contains(err.Error(), "decode capability payload") {
		t.Fatalf("SignedPayload error = %v, want a payload decoding failure", err)
	}
}

func TestDecodeScopeRefusesUnreadableClaims(t *testing.T) {
	t.Parallel()
	const good = "2026-08-12T12:00:00.000000000Z"
	t.Run("not json", func(t *testing.T) {
		t.Parallel()
		_, err := DecodeScope([]byte(`garbage`), "k1")
		requireError(t, err, "claims that are not JSON")
	})
	t.Run("missing times", func(t *testing.T) {
		t.Parallel()
		_, err := DecodeScope([]byte(`{}`), "k1")
		requireError(t, err, "claims without times")
	})
	for _, field := range []string{"nbf", "exp", "from", "until", "iat"} {
		t.Run("unreadable "+field, func(t *testing.T) {
			t.Parallel()
			claims := map[string]string{"nbf": good, "exp": good, "from": good, "until": good, "iat": good}
			claims[field] = "not a time"
			raw := `{"nbf":"` + claims["nbf"] + `","exp":"` + claims["exp"] + `","from":"` + claims["from"] + `","until":"` + claims["until"] + `","iat":"` + claims["iat"] + `"}`
			if _, err := DecodeScope([]byte(raw), "k1"); err == nil {
				t.Fatalf("a token with an unreadable %s decoded", field)
			}
		})
	}
}
