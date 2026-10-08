package domain

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/canonicaljson"
	"github.com/ghassan-ai-projects/agentic-stream/internal/contractsv1/contractstest"
)

type storedCase struct {
	name     string
	schema   SchemaName
	domain   canonicaljson.Domain
	document map[string]any
	tamper   string
}

func storedCases(t *testing.T) []storedCase {
	t.Helper()
	intent := map[string]any{
		"intent_id": "int-1", "decision_id": "dec-1", "tenant_id": "tenant", "situation_id": "sit-1", "situation_version": 1,
		"type": "maintenance.ticket", "risk_class": "R1", "parameters": map[string]any{"target": "motor/1"},
		"expires_at": "2026-10-06T13:00:00.000000000Z",
	}
	digest, err := IntentDigest(intent)
	if err != nil {
		t.Fatal(err)
	}
	intent["intent_digest"] = digest
	return []storedCase{
		{"command", SchemaCommand, canonicaljson.DomainCommand, map[string]any{
			"command_id": "cmd-1", "intent_id": "int-1", "tenant_id": "tenant", "effector_route": "maintenance.ticket",
			"normalized_target": "motor/1", "idempotency_key": "sha256:" + strings.Repeat("1", 64), "status": "prepared",
			"payload": map[string]any{"reason": "test"}, "created_at": "2026-10-06T12:00:00.000000000Z",
		}, "normalized_target"},
		{"intent", SchemaIntent, canonicaljson.DomainIntent, intent, "type"},
		{"decision", SchemaDecision, canonicaljson.DomainDecision, map[string]any{
			"decision_id": "dec-1", "episode_id": "epi-1", "attempt_id": "att-1", "fence": 1,
			"snapshot_digest": "sha256:" + strings.Repeat("0", 64), "situation_id": "sit-1", "situation_version": 1,
			"confidence": 0.9, "intents": []any{intent},
		}, "situation_id"},
		{"snapshot", SchemaSnapshot, canonicaljson.DomainSnapshot, map[string]any{
			"situation_id": "s1", "situation_version": 1, "situation_type": "motor_over_temp", "tenant_id": "default",
			"entity": map[string]any{"type": "motor", "id": "motor-1"}, "phase": "warning", "severity": 10.0,
			"completeness": "on_time", "event_horizon": "2026-01-01T00:00:00Z", "spec_digest": "sha256:" + strings.Repeat("2", 64),
			"facts": map[string]any{},
		}, "situation_type"},
	}
}

func sealedBody(t *testing.T, c storedCase, document map[string]any) ([]byte, []byte) {
	t.Helper()
	body := document
	if c.domain == canonicaljson.DomainIntent {
		body = withoutIntentDigest(document)
	}
	canonical, sum, err := canonicaljson.Seal(c.domain, body)
	if err != nil {
		t.Fatal(err)
	}
	if c.domain != canonicaljson.DomainIntent {
		return canonical, sum
	}
	canonical, err = canonicaljson.Marshal(document)
	if err != nil {
		t.Fatal(err)
	}
	return canonical, sum
}

func TestStoredDocumentRuleHoldsForEverySchemaAndDomain(t *testing.T) {
	t.Parallel()
	for _, c := range storedCases(t) {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			canonical, sum := sealedBody(t, c, c.document)
			for _, accepted := range [][]byte{canonical, indented(t, canonical)} {
				if _, err := VerifyStoredDocument(c.schema, c.domain, accepted, sum); err != nil {
					t.Fatalf("bound document refused: %v", err)
				}
			}
			tampered := cloneWith(c.document, c.tamper, "tampered")
			tamperedBytes, err := canonicaljson.Marshal(tampered)
			if err != nil {
				t.Fatal(err)
			}
			for name, tc := range map[string]struct {
				raw, sum []byte
				want     error
			}{
				"tampered body":              {tamperedBytes, sum, ErrDocumentDigest},
				"tampered digest":            {canonical, make([]byte, 32), ErrDocumentDigest},
				"short digest":               {canonical, sum[:31], ErrDocumentDigest},
				"no digest":                  {canonical, nil, ErrDocumentDigest},
				"schema invalid":             {[]byte(`{}`), sum, ErrDocumentSchema},
				"not json":                   {[]byte(`{`), sum, ErrDocumentJSON},
				"not an object":              {[]byte(`[]`), sum, ErrDocumentJSON},
				"null":                       {[]byte(`null`), sum, ErrDocumentJSON},
				"trailing value":             {append(append([]byte(nil), canonical...), []byte(` {}`)...), sum, ErrDocumentJSON},
				"duplicate key":              {contractstest.AmbiguousKeyJSON(canonical, c.tamper), sum, ErrDocumentJSON},
				"duplicate key mid-document": {bytes.Replace(canonical, []byte(`"`+c.tamper+`"`), []byte(`"`+c.tamper+`":"x","`+c.tamper+`"`), 1), sum, ErrDocumentJSON},
			} {
				if _, err := VerifyStoredDocument(c.schema, c.domain, tc.raw, tc.sum); !errors.Is(err, tc.want) {
					t.Errorf("%s: err = %v, want %v", name, err, tc.want)
				}
			}
		})
	}
}

func TestIntentDigestFieldMustAgreeWithTheStoredDigest(t *testing.T) {
	t.Parallel()
	c := storedCases(t)[1]
	canonical, sum := sealedBody(t, c, c.document)
	other := cloneWith(c.document, "intent_digest", "sha256:"+strings.Repeat("a", 64))
	otherBytes, err := canonicaljson.Marshal(other)
	if err != nil {
		t.Fatal(err)
	}
	missing := cloneWith(c.document, "intent_digest", nil)
	delete(missing, "intent_digest")
	missingBytes, err := canonicaljson.Marshal(missing)
	if err != nil {
		t.Fatal(err)
	}
	for name, raw := range map[string][]byte{"self digest differs": otherBytes, "self digest absent": missingBytes} {
		if _, err := VerifyStoredDocument(c.schema, c.domain, raw, sum); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
	if _, err := VerifyStoredDocument(c.schema, c.domain, canonical, sum); err != nil {
		t.Fatalf("agreeing intent refused: %v", err)
	}
}

func TestDecodeDocumentJSONRefusesWhatALenientReaderAccepts(t *testing.T) {
	t.Parallel()
	canonical := []byte(`{"a":1,"b":"x"}`)
	ambiguous := contractstest.AmbiguousKeyJSON(canonical, "b")
	var lenient map[string]any
	if err := json.Unmarshal(ambiguous, &lenient); err != nil || lenient["b"] != "x" {
		t.Fatalf("fixture is not ambiguous: %v %v", lenient, err)
	}
	if _, err := DecodeDocumentJSON(ambiguous); !errors.Is(err, ErrDocumentJSON) {
		t.Fatalf("err = %v, want ErrDocumentJSON", err)
	}
}

func indented(t *testing.T, canonical []byte) []byte {
	t.Helper()
	var out bytes.Buffer
	if err := json.Indent(&out, canonical, "", "  "); err != nil {
		t.Fatal(err)
	}
	return out.Bytes()
}

func cloneWith(document map[string]any, key string, value any) map[string]any {
	clone := make(map[string]any, len(document))
	for k, v := range document {
		clone[k] = v
	}
	clone[key] = value
	return clone
}
