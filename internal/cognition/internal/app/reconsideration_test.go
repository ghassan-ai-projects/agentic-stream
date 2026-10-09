package app

import (
	"database/sql"
	"errors"
	"strings"
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/cognition/internal/store"
)

func TestReconsiderationAdmissionIsReplayDeduplicated(t *testing.T) {
	t.Parallel()
	tests := map[string]correctionShape{
		"immediate predecessor":          immediatePredecessor,
		"latest command-bearing version": {versionCount: 4, commandVersion: 1, correctionVersion: 4, previousVersion: 3},
	}
	for name, shape := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			f := newReconsiderationFixture(t, shape)
			for range 2 {
				if err := f.process(); err != nil {
					t.Fatal(err)
				}
			}
			reconsiderations, items := f.count("SELECT COUNT(*) FROM reconsiderations"), f.count("SELECT COUNT(*) FROM scheduler_items WHERE kind = 'reconsider'")
			if reconsiderations != 1 || items != 1 {
				t.Fatalf("reconsiderations = %d, items = %d; want one of each after a replayed correction", reconsiderations, items)
			}
		})
	}
}

func TestAdmittedReconsiderationIsAnExplainedDeepLaneEvaluationWithAnAnnouncement(t *testing.T) {
	t.Parallel()
	f := newReconsiderationFixture(t, immediatePredecessor)
	if err := f.process(); err != nil {
		t.Fatal(err)
	}
	record, err := store.NewReader(f.db.DB).TriggerEvaluations(t.Context(), "tenant", "sit-reconsider", 2)
	if err != nil || len(record) != 1 {
		t.Fatalf("evaluations = %+v, %v", record, err)
	}
	if got := record[0]; got.Outcome != "admitted" || got.Lane != "deep" || got.TriggerName != "prior_action_invalidated" || !strings.Contains(strings.Join(got.Reasons, ";"), "accepted action invalidated") {
		t.Fatalf("evaluation = %+v", got)
	}
	if got := f.count("SELECT COUNT(*) FROM notifications WHERE event_id LIKE 'reconsideration.admitted:%'"); got != 1 {
		t.Fatalf("reconsideration notifications = %d, want 1", got)
	}
	if reasoned := f.count("SELECT last_reasoned_version FROM situations"); reasoned != 2 {
		t.Fatalf("last reasoned version = %d, want 2", reasoned)
	}
}

func TestReconsiderationFollowsOnlyTheLatestApprovedIntentAtOrBeforeThePreviousVersion(t *testing.T) {
	t.Parallel()
	f := newReconsiderationFixture(t, correctionShape{versionCount: 4, commandVersion: 1, correctionVersion: 4, previousVersion: 3})
	f.insertExecutedCommand("cmd-v2", "dec-v2", "epi-v2", "int-v2", 2, "approved")
	f.insertExecutedCommand("cmd-v2-denied", "dec-v2-denied", "epi-v2-denied", "int-v2-denied", 3, "denied")
	f.insertExecutedCommand("cmd-after", "dec-after", "epi-after", "int-after", 4, "approved")
	if err := f.process(); err != nil {
		t.Fatal(err)
	}
	if got := scalarString(f, "SELECT invalidated_command_id FROM reconsiderations"); got != "cmd-v2" || f.count("SELECT COUNT(*) FROM reconsiderations") != 1 {
		t.Fatalf("invalidated command = %q, want only cmd-v2: the latest approved intent at or before version 3", got)
	}
}

func scalarString(f *reconsiderationFixture, query string) string {
	f.t.Helper()
	var value string
	if err := f.db.QueryRowContext(f.t.Context(), query).Scan(&value); err != nil {
		f.t.Fatalf("%s: %v", query, err)
	}
	return value
}

func TestCorrectionsAreNotReconsideredUnlessThePolicySaysSo(t *testing.T) {
	t.Parallel()
	f := newReconsiderationFixture(t, immediatePredecessor)
	if err := f.processWith(f.newService("correct")); err != nil {
		t.Fatal(err)
	}
	if got := f.count("SELECT COUNT(*) FROM reconsiderations"); got != 0 {
		t.Fatalf("reconsiderations = %d, want none under the correct policy", got)
	}
}

func TestCorrectionWhoseSnapshotDigestDoesNotMatchIsRefused(t *testing.T) {
	t.Parallel()
	f := newReconsiderationFixture(t, immediatePredecessor)
	f.exec("UPDATE situation_versions SET snapshot_sha256 = zeroblob(32) WHERE version = 2")
	err := f.process()
	if err == nil || !strings.Contains(err.Error(), "correction snapshot digest mismatch") {
		t.Fatalf("error = %v, want a digest mismatch", err)
	}
	if got := f.count("SELECT COUNT(*) FROM reconsiderations"); got != 0 {
		t.Fatalf("reconsiderations = %d, want none", got)
	}
}

func TestReconsiderationRefusesASpecWithoutADigest(t *testing.T) {
	t.Parallel()
	f := newReconsiderationFixture(t, immediatePredecessor)
	service := f.newService("correct_and_reconsider")
	service.spec.Digest = ""
	err := f.processWith(service)
	if err == nil || !strings.Contains(err.Error(), "compiled spec has no digest") {
		t.Fatalf("error = %v, want a missing digest refusal", err)
	}
}

func TestReconsiderationRollsBackWithTheCallersTransaction(t *testing.T) {
	t.Parallel()
	f := newReconsiderationFixture(t, immediatePredecessor)
	rollback := errors.New("caller rollback")
	err := f.db.WithTx(t.Context(), func(tx *sql.Tx) error {
		if err := f.service.Process(t.Context(), store.Join(tx), f.current); err != nil {
			return err
		}
		return rollback
	})
	if !errors.Is(err, rollback) {
		t.Fatalf("error = %v, want the caller's rollback", err)
	}
	for _, table := range []string{"reconsiderations", "trigger_evaluations", "notifications"} {
		if got := f.count("SELECT COUNT(*) FROM " + table); got != 0 {
			t.Errorf("%s rows after rollback = %d, want none", table, got)
		}
	}
	if got := f.count("SELECT COUNT(*) FROM scheduler_items WHERE kind = 'reconsider'"); got != 0 {
		t.Errorf("reconsideration items after rollback = %d, want none", got)
	}
}

func TestUnreadablePriorDocumentsRejectOnlyThatReconsideration(t *testing.T) {
	t.Parallel()
	for name, corrupt := range map[string]string{
		"prior decision is not an object":   "UPDATE decisions SET raw_json = X'5B5D' WHERE decision_id = 'dec-bad'",
		"executed command is not json":      "UPDATE commands SET command_json = X'7B' WHERE command_id = 'cmd-bad'",
		"provider result is not valid json": "UPDATE outcomes SET provider_result_json = X'7B' WHERE command_id = 'cmd-bad'",
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			f := newReconsiderationFixture(t, immediatePredecessor)
			f.insertExecutedCommand("cmd-bad", "dec-bad", "epi-bad", "int-bad", 1, "approved")
			f.exec(corrupt)
			if err := f.process(); err != nil {
				t.Fatalf("a malformed historical row blocked the corrected version: %v", err)
			}
			admitted := f.count("SELECT COUNT(*) FROM reconsiderations WHERE invalidated_command_id = 'cmd-reconsider-1'")
			items := f.count("SELECT COUNT(*) FROM scheduler_items WHERE kind = 'reconsider'")
			rejected := f.count("SELECT COUNT(*) FROM trigger_evaluations WHERE trigger_name = 'prior_action_invalidated' AND outcome = 'rejected'")
			reasoned := f.count("SELECT last_reasoned_version FROM situations WHERE situation_id = 'sit-reconsider'")
			if admitted != 1 || items != 1 || rejected != 1 || reasoned != 2 {
				t.Fatalf("admitted=%d items=%d rejected=%d reasoned=%d; want the good command admitted, the bad one rejected and the version reasoned", admitted, items, rejected, reasoned)
			}
		})
	}
}
