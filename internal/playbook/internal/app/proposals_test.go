package app

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/draftport"
	"github.com/ghassan-ai-projects/agentic-stream/internal/playbook/internal/store"
	"github.com/ghassan-ai-projects/agentic-stream/internal/policy"
	"github.com/ghassan-ai-projects/agentic-stream/internal/sources"
	"github.com/ghassan-ai-projects/agentic-stream/internal/spec"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage/storagetest"
)

const pumpDigest = "sha256:17f5e82f53bc294d167f8543598ba2d49f5c60373985b78aa9ebe17288e79f19"

type cannedDrafter struct{ when string }

func (d cannedDrafter) Draft(_ context.Context, request draftport.Request) (draftport.Reply, error) {
	cited := make([]string, 0, len(request.Cases))
	for _, c := range request.Cases {
		cited = append(cited, c.EpisodeID)
	}
	return draftport.Reply{Proposal: &draftport.Proposal{DiagnosisCode: request.Key.DiagnosisCode, Intent: request.Key.Intent,
		Preset: request.Key.Preset, When: d.when, CitedEpisodeIDs: cited}, Usage: draftport.Usage{InputTokens: 900}}, nil
}

type flakyDrafter struct{ ids *[]string }

func (d flakyDrafter) Draft(ctx context.Context, request draftport.Request) (draftport.Reply, error) {
	*d.ids = append(*d.ids, request.DraftID)
	if len(*d.ids) == 1 {
		return draftport.Reply{}, errors.New("provider unavailable")
	}
	return cannedDrafter{when: "features.vibration_mean_15m > 3.9"}.Draft(ctx, request)
}

type proposalFixture struct {
	service *Service
	db      *storage.DB
	path    string
	key     ed25519.PrivateKey
}

func newProposalFixture(t *testing.T) proposalFixture {
	t.Helper()
	compiled, err := spec.CompileFile(t.Context(), "../../../../examples/pump-station/pump-station.situation.yaml")
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "site-a.playbook.yaml")
	copyPlaybook(t, path)
	book, err := spec.CompilePlaybookFile(compiled, path)
	if err != nil {
		t.Fatal(err)
	}
	db := storagetest.OpenTemp(t)
	service, err := New(Config{Store: store.New(db).WithSignatures(policySignatures{}), Spec: compiled, Playbook: book, PlaybookPath: path, TenantID: "t",
		Clock: fixedClock(), KindDigests: func() map[string]string { return map[string]string{"pump": pumpDigest} },
		Drafter: cannedDrafter{when: "features.vibration_mean_15m > 3.9"}})
	if err != nil {
		t.Fatal(err)
	}
	return proposalFixture{service: service, db: db, path: path, key: governWith(t, db)}
}

func copyPlaybook(t *testing.T, path string) {
	t.Helper()
	source, err := os.ReadFile("../../../../examples/pump-station/testdata/site-a.playbook.yaml")
	if err != nil {
		t.Fatal(err)
	}
	writeIn(t, path, source)
}

func writeIn(t *testing.T, path string, content []byte) {
	t.Helper()
	root, err := os.OpenRoot(filepath.Dir(path))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = root.Close() }()
	if err := root.WriteFile(filepath.Base(path), content, 0o600); err != nil {
		t.Fatal(err)
	}
}

func governWith(t *testing.T, db *storage.DB) ed25519.PrivateKey {
	t.Helper()
	public, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	document, err := policy.ParsePrincipals([]byte("tenant: t\nprincipals:\n  - id: relay\n  - id: operator\n    public_key: " +
		base64.StdEncoding.EncodeToString(public) + "\nroles:\n  - id: pumps\n    name: Pumps\n    members: [operator]\n    authorities:\n      - entity: site-a:pump-1\n        risks: [R1]\n"))
	if err != nil {
		t.Fatal(err)
	}
	fence := policy.Ownership{Check: func(context.Context, *sql.Tx, string) error { return nil }, Epoch: "test"}
	if err := db.WithTx(t.Context(), func(tx *sql.Tx) error {
		_, err := policy.ApplyPrincipals(t.Context(), tx, fence, document, time.Now())
		return err
	}); err != nil {
		t.Fatal(err)
	}
	return private
}

func (f proposalFixture) seedVerifiedDriftCases(t *testing.T, count int) {
	t.Helper()
	for i := range count {
		if _, err := f.db.ExecContext(t.Context(), `INSERT INTO case_verifications (situation_id, tenant_id, entity_id, situation_type,
			episode_id, decision_id, situation_version, diagnosis_code, kind_digest, intent, preset, rule_id, rule_version, acted_at,
			window_ends_at, features_json, summary, verdict, reason, created_at) VALUES (?, 't', 'site-a:pump-1', 'pump_condition', ?, ?, 1,
			'sensor_drift', ?, 'schedule_maintenance', 'sensor_drift', '', 0, ?, ?, CAST(? AS BLOB), 'probe drift', 'verified', 'ok', 'x')`,
			"s"+string(rune('a'+i)), "e"+string(rune('a'+i)), "d"+string(rune('a'+i)), pumpDigest,
			"2026-01-0"+string(rune('1'+i))+"T06:00:00.000000000Z", "2026-01-0"+string(rune('2'+i))+"T06:00:00.000000000Z",
			`{"vibration_mean_15m":4.`+string(rune('0'+i))+`,"vibration_mean_24h":3.0,"current_mean_15m":30.0}`); err != nil {
			t.Fatal(err)
		}
	}
}

func (f proposalFixture) inTx(t *testing.T, use func(*store.Tx) error) {
	t.Helper()
	if err := f.service.Store().WithTx(t.Context(), use); err != nil {
		t.Fatal(err)
	}
}

func (f proposalFixture) pendingProposalID(t *testing.T) string {
	t.Helper()
	var id string
	if err := f.db.QueryRowContext(t.Context(), "SELECT proposal_id FROM rule_proposals WHERE status = 'pending'").Scan(&id); err != nil {
		t.Fatalf("no pending proposal: %v", err)
	}
	return id
}

func TestVerifiedCasesBecomeAProposalThatAnApprovalWritesInShadow(t *testing.T) {
	t.Parallel()
	f := newProposalFixture(t)
	f.seedVerifiedDriftCases(t, 3)
	if err := f.service.Settle(t.Context()); err != nil {
		t.Fatal(err)
	}
	if drafted, err := f.service.Draft(t.Context()); err != nil || !drafted {
		t.Fatalf("draft = %v, %v; want one draft run", drafted, err)
	}
	id := f.pendingProposalID(t)
	lookup := ProposalLookup{ID: id, Approver: "operator", Relay: "relay", Approved: true}
	var presented ProposalPresentation
	f.inTx(t, func(tx *store.Tx) (err error) {
		presented, err = f.service.PresentProposal(t.Context(), tx, lookup)
		return err
	})
	if presented.Proposal.Replay.SourceCases != 3 || presented.Proposal.Rule.Mode != spec.RuleModeShadow {
		t.Fatalf("presented proposal = %+v", presented.Proposal)
	}
	resolution := ProposalResolution{ProposalLookup: lookup, Signature: ed25519.Sign(f.key, presented.SigningBytes)}
	var result ProposalResult
	f.inTx(t, func(tx *store.Tx) (err error) {
		result, err = f.service.ResolveProposal(t.Context(), tx, resolution)
		return err
	})
	written, err := spec.CompilePlaybookFile(f.service.rules.Spec(), f.path)
	if err != nil || result.Status != "written" || written.Digest != result.WrittenDigest || len(written.Rules) != 3 {
		t.Fatalf("result %+v, written %v (%v); want the rule written", result, written, err)
	}
	if rule := written.Rules[2]; rule.Mode != spec.RuleModeShadow || rule.Provenance.ApprovalDigest == "" || rule.PinnedKind.Name != "pump" ||
		rule.ShadowExit.Agreeing != 3 {
		t.Fatalf("written rule = %+v, want an approved shadow rule pinned to the pump, leaving shadow after the spec's 3 agreeing matches", rule)
	}
}

func TestARetriedDraftReachesTheWorkerUnderANewID(t *testing.T) {
	t.Parallel()
	f := newProposalFixture(t)
	var ids []string
	f.service.drafter = flakyDrafter{ids: &ids}
	f.seedVerifiedDriftCases(t, 3)
	if err := f.service.Settle(t.Context()); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if _, err := f.service.Draft(t.Context()); err != nil {
			t.Fatal(err)
		}
	}
	if len(ids) != 2 || ids[0] == ids[1] || !strings.HasSuffix(ids[0], "-a1") || !strings.HasSuffix(ids[1], "-a2") {
		t.Fatalf("draft ids = %v, want one per attempt", ids)
	}
	f.pendingProposalID(t)
}

func TestAForgedOrRepeatedApprovalIsRefused(t *testing.T) {
	t.Parallel()
	f := newProposalFixture(t)
	f.seedVerifiedDriftCases(t, 3)
	if err := f.service.Settle(t.Context()); err != nil {
		t.Fatal(err)
	}
	if _, err := f.service.Draft(t.Context()); err != nil {
		t.Fatal(err)
	}
	lookup := ProposalLookup{ID: f.pendingProposalID(t), Approver: "operator", Relay: "relay"}
	forged := ProposalResolution{ProposalLookup: lookup, Signature: []byte("not a signature")}
	err := f.service.Store().WithTx(t.Context(), func(tx *store.Tx) error {
		_, err := f.service.ResolveProposal(t.Context(), tx, forged)
		return err
	})
	if !errors.Is(err, policy.ErrApprovalUnauthorized) {
		t.Fatalf("a forged signature = %v, want unauthorized", err)
	}
	stranger := lookup
	stranger.Approver = "stranger"
	err = f.service.Store().WithTx(t.Context(), func(tx *store.Tx) error {
		_, err := f.service.PresentProposal(t.Context(), tx, stranger)
		return err
	})
	if !errors.Is(err, policy.ErrApprovalUnauthorized) {
		t.Fatalf("an unknown approver = %v, want unauthorized", err)
	}
}

func TestTheWriterRefusesAPlaybookChangedUnderneath(t *testing.T) {
	t.Parallel()
	f := newProposalFixture(t)
	source, err := os.ReadFile(f.path)
	if err != nil {
		t.Fatal(err)
	}
	writeIn(t, f.path, []byte(strings.Replace(string(source), "rateCapPerHour: 4", "rateCapPerHour: 5", 1)))
	if _, err := f.service.writeRule(spec.Rule{}); !errors.Is(err, ErrPlaybookChanged) {
		t.Fatalf("write over a hand-edited playbook = %v, want refused", err)
	}
}

func TestAShadowRuleWhoseExitIsMetIsWrittenActive(t *testing.T) {
	t.Parallel()
	f := newProposalFixture(t)
	for i := range 10 {
		if _, err := f.db.ExecContext(t.Context(), `INSERT INTO rule_evaluations (evaluation_id, tenant_id, rule_id, rule_version,
			playbook_digest, kind, situation_id, situation_version, in_envelope, recommendation_json, verdict, event_time, created_at)
			VALUES (?, 't', 'misalignment', 1, 'pb', 'shadow', 's', 1, 1, CAST('{}' AS BLOB), 'agree', 'x', 'x')`, "ev"+string(rune('a'+i))); err != nil {
			t.Fatal(err)
		}
	}
	if promoted, err := f.service.Promote(t.Context()); err != nil || promoted != 1 {
		t.Fatalf("promote = %d, %v; want the misalignment rule", promoted, err)
	}
	written, err := spec.CompilePlaybookFile(f.service.rules.Spec(), f.path)
	if err != nil || written.Rules[1].Mode != spec.RuleModeActive {
		t.Fatalf("written playbook = %+v (%v), want misalignment active", written, err)
	}
}

func fixedClock() *sources.Virtual {
	return sources.NewVirtual(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
}

type policySignatures struct{}

func (policySignatures) CheckSigner(ctx context.Context, tx *sql.Tx, approval store.SignedApproval) error {
	return policy.CheckApprovalSigner(ctx, tx, policy.SignedApproval(approval))
}

func (policySignatures) Verify(ctx context.Context, tx *sql.Tx, approval store.SignedApproval) (string, error) {
	return policy.VerifySignedApproval(ctx, tx, policy.SignedApproval(approval))
}
