package domain

import (
	"errors"
	"strings"
	"testing"
	"time"
)

var registeredRejectionReasons = []RejectionReason{
	RejectUnknownEpisode, RejectStaleAttempt, RejectWrongAttempt, RejectTerminalAttempt, RejectEpisodeClosed,
	RejectSchemaInvalid, RejectSnapshotMismatch, RejectEvidenceNotVisible, RejectForgedReference, RejectOversized,
	RejectExpired, RejectIntentTypeNotAllowed, RejectRiskCeilingExceeded, RejectCatalogMissing, RejectCatalogForged,
	RejectIntentTypeNotInCatalog, RejectRiskLabelMismatch, RejectParameterSchemaViolated, RejectPresetMismatch,
	RejectUngroundedEvidence,
}

func TestEveryRegisteredRejectionReasonIsAccepted(t *testing.T) {
	t.Parallel()
	for _, reason := range registeredRejectionReasons {
		if err := CheckRejectionReason(reason); err != nil {
			t.Errorf("registered reason %q refused: %v", reason, err)
		}
	}
}

func TestAnUnregisteredRejectionReasonIsRefusedByName(t *testing.T) {
	t.Parallel()
	for _, reason := range []RejectionReason{"made_up", "", "STALE_ATTEMPT"} {
		err := CheckRejectionReason(reason)
		if err == nil || !strings.Contains(err.Error(), `invalid rejection reason "`+string(reason)+`"`) {
			t.Errorf("CheckRejectionReason(%q) = %v, want a refusal naming the reason", reason, err)
		}
	}
}

func TestRejectionDetailsDefaultToAnEmptyJSONObject(t *testing.T) {
	t.Parallel()
	if got := string(RejectionDetails(nil)); got != "{}" {
		t.Errorf("nil details = %s, want {}", got)
	}
	if got := string(RejectionDetails([]byte{})); got != "{}" {
		t.Errorf("empty details = %s, want {}", got)
	}
	if got := string(RejectionDetails([]byte(`{"a":1}`))); got != `{"a":1}` {
		t.Errorf("given details = %s, want them unchanged", got)
	}
}

func TestRejectionIDDependsOnEveryInputAndIsStable(t *testing.T) {
	t.Parallel()
	identity := Identity{EpisodeID: "e", AttemptID: "a", Fence: 3}
	instant := time.Date(2026, 8, 12, 12, 0, 0, 0, time.UTC)
	base := RejectionID(identity, RejectStaleAttempt, []byte("{}"), instant)
	if !strings.HasPrefix(base, "rej_") || base != RejectionID(identity, RejectStaleAttempt, []byte("{}"), instant) {
		t.Fatalf("id %s is not a stable rej_ identity", base)
	}
	other := map[string]string{
		"episode": RejectionID(Identity{EpisodeID: "x", AttemptID: "a", Fence: 3}, RejectStaleAttempt, []byte("{}"), instant),
		"attempt": RejectionID(Identity{EpisodeID: "e", AttemptID: "x", Fence: 3}, RejectStaleAttempt, []byte("{}"), instant),
		"fence":   RejectionID(Identity{EpisodeID: "e", AttemptID: "a", Fence: 4}, RejectStaleAttempt, []byte("{}"), instant),
		"reason":  RejectionID(identity, RejectWrongAttempt, []byte("{}"), instant),
		"details": RejectionID(identity, RejectStaleAttempt, []byte(`{"x":1}`), instant),
		"instant": RejectionID(identity, RejectStaleAttempt, []byte("{}"), instant.Add(time.Nanosecond)),
	}
	for input, id := range other {
		if id == base {
			t.Errorf("id ignores the %s", input)
		}
	}
}

func TestRejectionIDIsPinnedForWholeSecondAndFractionalInstants(t *testing.T) {
	t.Parallel()
	identity := Identity{EpisodeID: "e", AttemptID: "a", Fence: 1}
	for name, test := range map[string]struct {
		at   time.Time
		want string
	}{
		"whole second": {time.Date(2026, 8, 12, 12, 0, 0, 0, time.UTC), "rej_151d5295a1b297ed669b5b3b0c68e26c8b94c55475de86f1ac6076568f7641ed"},
		"fractional":   {time.Date(2026, 8, 12, 12, 0, 0, 500_000_000, time.UTC), "rej_87cea6ff9879c8ab4ad11791c6b47330073a9edc0708a1c87af4df19ec0a5beb"},
	} {
		if got := RejectionID(identity, RejectStaleAttempt, []byte("{}"), test.at); got != test.want {
			t.Errorf("%s: id = %s, want %s", name, got, test.want)
		}
	}
}

func TestAnIdentityRefusalNamesItsReasonAndMatchesOnlyThatReason(t *testing.T) {
	t.Parallel()
	err := Refuse(RejectStaleAttempt)
	if got := err.Error(); got != "worker identity rejected: stale_attempt" {
		t.Errorf("message = %q", got)
	}
	wrapped := errors.Join(errors.New("context"), err)
	if !IsIdentityReason(wrapped, RejectStaleAttempt) || IsIdentityReason(wrapped, RejectWrongAttempt) {
		t.Error("IsIdentityReason must match the wrapped refusal's reason and no other")
	}
	if IsIdentityReason(errors.New("plain"), RejectStaleAttempt) || IsIdentityReason(nil, RejectStaleAttempt) {
		t.Error("IsIdentityReason matched an error that is not a worker refusal")
	}
}
