package domain

import (
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/kernel"
)

func TestValidateRefusesIntentsBoundToAnotherDecisionOrSituation(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		mutate func(map[string]any)
		reason string
		field  string
	}{
		{"decision", func(i map[string]any) { i["decision_id"] = "dec-other" }, "snapshot_mismatch", "intent.decision_id"},
		{"tenant", func(i map[string]any) { i["tenant_id"] = "tenant-other" }, "snapshot_mismatch", "intent.tenant_id"},
		{"situation", func(i map[string]any) { i["situation_id"] = "sit-other" }, "snapshot_mismatch", "intent.situation_id"},
		{"situation version", func(i map[string]any) { i["situation_version"] = 3 }, "snapshot_mismatch", "intent.situation_version"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			document := validDecision()
			test.mutate(firstIntent(document))
			resealIntents(document)
			_, err := validateDocument(t, document, validInput(t))
			requireRejection(t, err, test.reason, test.field)
		})
	}
}

func TestValidateRefusesDuplicateIntentIDs(t *testing.T) {
	t.Parallel()
	document := validDecision()
	watch := validIntent("int-1")
	watch["type"] = "install_watch_condition"
	document["intents"] = []any{validIntent("int-1"), watch}
	resealIntents(document)
	_, err := validateDocument(t, document, inputAllowing(t, "create_ticket", "install_watch_condition"))
	requireRejection(t, err, "schema_invalid", "intent_id")
}

func TestValidateIntentLifetimeEndsAtExpiresAt(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		expiresAt  time.Time
		wantReject bool
	}{
		{"future", validationTime.Add(time.Nanosecond), false},
		{"exactly now", validationTime, true},
		{"past", validationTime.Add(-time.Hour), true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			document := validDecision()
			firstIntent(document)["expires_at"] = kernel.FormatTime(test.expiresAt)
			resealIntents(document)
			_, err := validateDocument(t, document, validInput(t))
			if !test.wantReject {
				if err != nil {
					t.Fatalf("Validate: %v", err)
				}
				return
			}
			requireRejection(t, err, "expired", "intent.expires_at")
		})
	}
}

func TestValidateNormalizesIntentExpiryToUTC(t *testing.T) {
	t.Parallel()
	document := validDecision()
	firstIntent(document)["expires_at"] = "2026-08-12T13:00:00+02:00"
	resealIntents(document)
	result, err := validateDocument(t, document, validInput(t))
	if err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if want := validationTime.Add(time.Hour); !result.Intents[0].ExpiresAt.Equal(want) || result.Intents[0].ExpiresAt.Location() != time.UTC {
		t.Fatalf("expires at %v, want %v in UTC", result.Intents[0].ExpiresAt, want)
	}
}

func TestValidateAllowsOneActionableIntentBesideWatchesAndCompensations(t *testing.T) {
	t.Parallel()
	watch := validIntent("int-watch")
	watch["type"] = "install_watch_condition"
	compensation := validIntent("int-undo")
	compensation["type"] = "downgrade_maintenance_ticket"
	compensation["compensates"] = "cmd-original"
	tests := []struct {
		name    string
		intents []any
		wantErr bool
	}{
		{"actionable and watch", []any{validIntent("int-1"), watch}, false},
		{"actionable and compensation", []any{validIntent("int-1"), compensation}, false},
		{"two actionable", []any{validIntent("int-1"), validIntent("int-2")}, true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			document := validDecision()
			document["intents"] = test.intents
			resealIntents(document)
			input := inputAllowing(t, "create_ticket", "install_watch_condition")
			input.Reconsider = true
			_, err := validateDocument(t, document, input)
			if test.wantErr {
				requireRejection(t, err, "schema_invalid", "intents")
				return
			}
			if err != nil {
				t.Fatalf("Validate: %v", err)
			}
		})
	}
}
