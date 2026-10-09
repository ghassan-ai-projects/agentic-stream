package domain

import "testing"

func TestIntentDigestExcludesItselfAndVerifies(t *testing.T) {
	t.Parallel()

	document := map[string]any{"intent_id": "int-1", "type": "inspect", "parameters": map[string]any{"depth": 2}}
	digest, err := IntentDigest(document)
	if err != nil {
		t.Fatal(err)
	}
	document["intent_digest"] = digest
	if again, err := IntentDigest(document); err != nil || again != digest {
		t.Fatalf("digest must ignore its own field: %q, %v", again, err)
	}
	if !VerifyIntentDigest(document) {
		t.Fatal("valid digest did not verify")
	}

	tampered := map[string]any{"intent_id": "int-1", "type": "shutdown", "parameters": map[string]any{"depth": 2}, "intent_digest": digest}
	if VerifyIntentDigest(tampered) {
		t.Fatal("tampered intent verified")
	}
	if VerifyIntentDigest(map[string]any{"intent_id": "int-1"}) {
		t.Fatal("intent without a digest verified")
	}
	if VerifyIntentDigest(map[string]any{"intent_digest": 42}) {
		t.Fatal("non-string digest verified")
	}
}

func TestIntentDigestGolden(t *testing.T) {
	t.Parallel()

	document := map[string]any{"intent_id": "int-1", "type": "inspect", "parameters": map[string]any{"depth": 2}}
	got, err := IntentDigest(document)
	if err != nil {
		t.Fatal(err)
	}
	const want = "sha256:6891d331034c980e2329ff5989c91a9872bee0035c0d9c4d478026651e62b6f1"
	if got != want {
		t.Fatalf("IntentDigest = %s, want the frozen %s", got, want)
	}
}
