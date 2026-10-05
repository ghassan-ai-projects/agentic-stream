package domain

import (
	"errors"
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/canonicaljson"
)

func TestValidationRejectsAttemptBeforeSnapshotMismatch(t *testing.T) {
	t.Parallel()
	for _, field := range []string{"attempt_id", "fence"} {
		t.Run(field, func(t *testing.T) {
			document := validDecision()
			document["snapshot_digest"] = "sha256:" + ones(64)
			if field == "attempt_id" {
				document[field] = "att-other"
			} else {
				document[field] = 99
			}
			raw, err := canonicaljson.Marshal(document)
			if err != nil {
				t.Fatal(err)
			}
			digest, err := canonicaljson.Digest(canonicaljson.DomainDecision, document)
			if err != nil {
				t.Fatal(err)
			}
			_, err = Validate(raw, digest, validInput())
			var rejected *ValidationError
			if !errors.As(err, &rejected) || rejected.Reason != "stale_attempt" || rejected.Details["field"] != field {
				t.Fatalf("rejection = %v, want stale %s", err, field)
			}
		})
	}
}
