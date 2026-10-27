package store

import (
	"errors"
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/draftport"
	"github.com/ghassan-ai-projects/agentic-stream/internal/playbook/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/policy"
	"github.com/ghassan-ai-projects/agentic-stream/internal/spec"
)

var wearKey = domain.Key{DiagnosisCode: "bearing_wear", KindDigest: "sha256:pump", Intent: "schedule_maintenance", Preset: "bearing_wear"}

func TestADraftIsRequestedRunAndFinishedOncePerKeyAndEvidence(t *testing.T) {
	t.Parallel()
	s := seededCase(t)
	draft := domain.NewDraft("t", wearKey, 3)
	inTx(t, s, func(tx *Tx) error {
		for range 2 {
			if err := tx.RequestDraft(t.Context(), draft, storeNow); err != nil {
				return err
			}
		}
		history, err := tx.KeyHistory(t.Context(), "t", wearKey)
		if err != nil || !history.Pending {
			t.Fatalf("history = %+v, %v; want a pending draft", history, err)
		}
		next, found, err := tx.NextDraft(t.Context(), "t")
		if err != nil || !found || next.DraftID != draft.DraftID || next.EvidenceCases != 3 {
			t.Fatalf("next draft = %+v %v %v", next, found, err)
		}
		if err := tx.StartDraft(t.Context(), draft.DraftID, draftport.Request{DraftID: draft.DraftID}, storeNow); err != nil {
			return err
		}
		return tx.FinishDraft(t.Context(), draft.DraftID, domain.DraftNone, "no pattern", draftport.Reply{NoProposal: "no pattern"}, storeNow)
	})
	inTx(t, s, func(tx *Tx) error {
		if _, found, err := tx.NextDraft(t.Context(), "t"); err != nil || found {
			t.Fatalf("a finished draft is still next: %v %v", found, err)
		}
		cases, err := tx.KeyedCases(t.Context(), "t")
		if err != nil || len(cases) != 0 {
			t.Fatalf("settled cases = %+v, %v; the seeded case is still pending", cases, err)
		}
		return nil
	})
}

func TestAProposalIsRecordedReadAndResolvedOnce(t *testing.T) {
	t.Parallel()
	s := seededCase(t)
	draft := domain.NewDraft("t", wearKey, 3)
	proposal, err := domain.NewProposal(draft, spec.Rule{ID: "wear", Version: 1}, "R1", domain.ReplayReport{SourceCases: 3}, draftport.Proposal{Rationale: "worn"})
	if err != nil {
		t.Fatal(err)
	}
	inTx(t, s, func(tx *Tx) error {
		if err := tx.RequestDraft(t.Context(), draft, storeNow); err != nil {
			return err
		}
		return tx.RecordProposal(t.Context(), proposal, storeNow)
	})
	inTx(t, s, func(tx *Tx) error {
		read, err := tx.Proposal(t.Context(), "t", proposal.ProposalID)
		if err != nil || read.Digest != proposal.Digest || read.Replay.SourceCases != 3 || read.Status != domain.ProposalPending {
			t.Fatalf("read proposal = %+v, %v", read, err)
		}
		if _, err := tx.Proposal(t.Context(), "t", "rprop_missing"); !errors.Is(err, domain.ErrProposalNotFound) {
			t.Fatalf("a missing proposal = %v, want not found", err)
		}
		resolution := Resolution{ProposalID: proposal.ProposalID, Status: domain.ProposalRejected, Approver: "operator", Relay: "relay", Reason: "too broad"}
		if err := tx.ResolveProposal(t.Context(), resolution, storeNow); err != nil {
			return err
		}
		if err := tx.ResolveProposal(t.Context(), resolution, storeNow); !errors.Is(err, domain.ErrProposalResolved) {
			t.Fatalf("a second resolution = %v, want already resolved", err)
		}
		history, err := tx.KeyHistory(t.Context(), "t", wearKey)
		if err != nil || !history.Rejected || history.RejectedEvidence != 3 {
			t.Fatalf("history = %+v, %v; want the rejection with its evidence", history, err)
		}
		return nil
	})
}

func TestReplayHistoryAndShadowTalliesAreRead(t *testing.T) {
	t.Parallel()
	s := seededCase(t)
	inTx(t, s, func(tx *Tx) error {
		versions, err := tx.ReplayVersions(t.Context(), "t", "pump_condition", 10)
		if err != nil || len(versions) != 2 || versions[0].Decision != nil || versions[0].Facts["facts.vibration_mean_15m"] != 4.2 {
			t.Fatalf("versions = %+v, %v; want both versions and no model decision (the rule decided)", versions, err)
		}
		agree, disagree, err := tx.ShadowTallies(t.Context(), domain.RuleKey{ID: "bearing-wear", Version: 1})
		if err != nil || agree != 0 || disagree != 0 {
			t.Fatalf("tallies = %d/%d, %v", agree, disagree, err)
		}
		if err := tx.CheckSigner(t.Context(), "t", "operator", "relay", "R1"); !errors.Is(err, policy.ErrApprovalUnauthorized) {
			t.Fatalf("an unknown signer = %v, want unauthorized", err)
		}
		if _, err := tx.VerifySignature(t.Context(), "t", "operator", "relay", "R1", []byte("m"), []byte("s")); !errors.Is(err, policy.ErrApprovalUnauthorized) {
			t.Fatalf("an unknown signer's signature = %v, want unauthorized", err)
		}
		return nil
	})
}
