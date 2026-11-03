package domain_test

import (
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/plantwire/internal/domain"
)

var receivedAt = time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC)

func damperKind() domain.Kind {
	return domain.Kind{
		"kind": "damper",
		"metrics": []any{
			map[string]any{"name": "opening", "data_type": "number", "resolution": json.Number("1"), "writable": true},
			map[string]any{"name": "mode", "data_type": "enum", "enum_values": []any{"auto", "hand"}, "writable": true},
			map[string]any{"name": "flow", "data_type": "number", "writable": false},
		},
		"commands": []any{map[string]any{"name": "set_opening", "risk_class": "R1", "safe_stop": false}},
	}
}

func damperInventory(t *testing.T) domain.Inventory {
	t.Helper()
	inventory, err := domain.Commission([]domain.Kind{damperKind()}, nil)
	if err != nil {
		t.Fatal(err)
	}
	return inventory
}

func damperConversation(t *testing.T) *domain.Conversation {
	t.Helper()
	admission := domain.Admission{Environment: domain.EnvironmentSimulated, ContractDigest: contractDigest, Inventory: damperInventory(t)}
	conversation := domain.NewConversation(admission, 1, domain.Mapping{})
	birth := birthOf(t, func(b *domain.Birth) {
		b.Kinds = []domain.Kind{announced(t, damperKind())}
		b.Boards = []domain.Board{{Board: "board-01", Environment: domain.EnvironmentSimulated, Members: []string{"D1"}}}
	})
	conversation.Receive(domain.Arrival{Envelope: birth.Envelope, Sequenced: true, Birth: &birth})
	device := domain.DeviceBirth{Device: "D1", Kind: "damper", Board: "board-01", BootID: "b1", BirthTimeNS: startNS,
		Metrics: []domain.BirthMetric{{Name: "opening", Alias: 1, Value: json.Number("10"), Quality: "good"}, {Name: "mode", Alias: 2, Quality: "warming_up"}}}
	conversation.Receive(received(arrival(domain.TypeDeviceBirth, 1, func(a *domain.Arrival) { a.DeviceBirth = &device })))
	return conversation
}

func received(a domain.Arrival) domain.Arrival {
	a.ReceivedAt = receivedAt
	return a
}

func damperOrder(id string, args map[string]any) domain.CommandOrder {
	return domain.CommandOrder{CommandID: id, IdempotencyKey: contractDigest, PolicyDigest: contractDigest, Device: "D1",
		Command: "set_opening", Args: args, ExpectedBootID: "b1", DeadlineNS: startNS}
}

func succeeded(t *testing.T, conversation *domain.Conversation, seq uint64, order domain.CommandOrder) domain.Reaction {
	t.Helper()
	if _, err := conversation.Prepare(order); err != nil {
		t.Fatal(err)
	}
	conversation.Register(order)
	conversation.Receive(answer(seq, domain.TypeReceipt, order.CommandID, "accepted", "b1"))
	return conversation.Receive(answer(seq+1, domain.TypeResult, order.CommandID, "succeeded", "b1"))
}

func opening(seq uint64, boot string, historical bool, quality string, value any) domain.Arrival {
	return data(seq, "D1", boot, historical, domain.Sample{Alias: 1, Value: value, Quality: quality, EventTimeNS: startNS})
}

func evidenceOf(t *testing.T, conversation *domain.Conversation, seq uint64) []map[string]any {
	t.Helper()
	replies := conversation.Transmit(conversation.Receive(syncAt(seq, startNS)).Replies)
	if len(replies) != 1 || replies[0]["type"] != "HSYNC" {
		t.Fatalf("NSYNC replies = %v, want one HSYNC", replies)
	}
	entries, _ := replies[0]["command_evidence"].([]map[string]any)
	return entries
}

func TestTheArgumentsNamingWritableMetricsAreTheExpectedReadBack(t *testing.T) {
	t.Parallel()
	inventory := damperInventory(t)
	expected := inventory.Expectations("damper", map[string]any{"opening": 40.0, "mode": "auto", "lease_ms": 5000, "flow": 3.0})
	if len(expected) != 2 || expected[0].Metric != "opening" || expected[0].Tolerance != 1 || expected[1].Metric != "mode" || expected[1].Value != "auto" {
		t.Fatalf("Expectations = %+v, want opening within 1 and mode, no lease_ms and no read-only flow", expected)
	}
	if outputs := inventory.Outputs("DAMPER"); len(outputs) != 2 || outputs[0] != "opening" || outputs[1] != "mode" {
		t.Fatalf("Outputs = %v, want the writable metrics", outputs)
	}
	if inventory.Outputs("pump") != nil || inventory.Expectations("pump", map[string]any{"opening": 1}) != nil {
		t.Fatal("a kind no file commissions has outputs")
	}
	cases := []struct {
		expectation domain.Expectation
		value       any
		matches     bool
	}{
		{expected[0], json.Number("40.9"), true},
		{expected[0], json.Number("41.5"), false},
		{expected[0], "40", false},
		{expected[1], "auto", true},
		{expected[1], "hand", false},
		{expected[1], json.Number("1"), false},
		{domain.Expectation{Metric: "opening", Value: 40, Tolerance: 0}, int64(40), true},
	}
	for _, tc := range cases {
		if got := tc.expectation.Matches(tc.value); got != tc.matches {
			t.Errorf("%+v.Matches(%v) = %v, want %v", tc.expectation, tc.value, got, tc.matches)
		}
	}
}

func TestOnlyAGoodLiveReadingOnTheBootAfterTheResultVerifiesACommand(t *testing.T) {
	t.Parallel()
	conversation := damperConversation(t)
	conversation.Receive(opening(2, "b1", false, "good", json.Number("40")))
	if reaction := succeeded(t, conversation, 3, damperOrder("c-1", map[string]any{"opening": 40.0})); len(reaction.Verifications) != 0 {
		t.Fatalf("a reading before the result verified the command: %+v", reaction.Verifications)
	}
	for seq, sample := range []domain.Arrival{opening(5, "b1", true, "good", json.Number("40")), opening(6, "b1", false, "suspect", json.Number("40")),
		opening(7, "b0", false, "good", json.Number("40")), opening(8, "b1", false, "good", json.Number("30"))} {
		if reaction := conversation.Receive(sample); len(reaction.Verifications) != 0 {
			t.Fatalf("sample %d verified the command: %+v", seq, reaction.Verifications)
		}
	}
	verifications := conversation.Receive(opening(9, "b1", false, "good", json.Number("40.5"))).Verifications
	if len(verifications) != 1 || !verifications[0].Verified || verifications[0].Observations[0].Observed != json.Number("40.5") {
		t.Fatalf("Verifications = %+v, want c-1 verified by 40.5", verifications)
	}
	feedback := verifications[0].Feedback()
	if feedback["command_id"] != "c-1" || feedback["boot_id"] != "b1" || len(feedback["readings"].([]any)) != 1 {
		t.Fatalf("Feedback = %v", feedback)
	}
	entries := evidenceOf(t, conversation, 10)
	if len(entries) != 1 || entries[0]["outcome"] != "observed" || entries[0]["observed_value"] != json.Number("40.5") || entries[0]["expected"] != 40.0 {
		t.Fatalf("HSYNC command_evidence = %v, want c-1 observed at 40.5", entries)
	}
	if again := evidenceOf(t, conversation, 11); again != nil {
		t.Fatalf("the evidence was sent twice: %v", again)
	}
}

func TestAWithdrawnWatchIsReportedNotObservedAndWordsCarryNoEvidence(t *testing.T) {
	t.Parallel()
	conversation := damperConversation(t)
	succeeded(t, conversation, 2, damperOrder("c-1", map[string]any{"opening": 40.0, "mode": "auto"}))
	conversation.Receive(data(4, "D1", "b1", false, domain.Sample{Alias: 2, Value: "auto", Quality: "good", EventTimeNS: startNS}))
	conversation.Unwatch("c-1")
	conversation.Unwatch("c-unknown")
	entries := evidenceOf(t, conversation, 5)
	if len(entries) != 1 || entries[0]["metric"] != "opening" || entries[0]["outcome"] != "not_observed" {
		t.Fatalf("HSYNC command_evidence = %v, want opening not observed and nothing for the word", entries)
	}
	if verifications := conversation.Receive(opening(6, "b1", false, "good", json.Number("40"))).Verifications; len(verifications) != 0 {
		t.Fatalf("a withdrawn watch verified: %+v", verifications)
	}
}

func TestACommandWithNothingToReadBackIsUnverifiedAtOnce(t *testing.T) {
	t.Parallel()
	conversation := damperConversation(t)
	verifications := succeeded(t, conversation, 2, damperOrder("c-1", map[string]any{"lease_ms": 5000})).Verifications
	if len(verifications) != 1 || verifications[0].Verified || verifications[0].CommandID != "c-1" {
		t.Fatalf("Verifications = %+v, want c-1 unverified at once", verifications)
	}
	if _, err := conversation.Prepare(damperOrder("c-2", map[string]any{"opening": 40.0})); err != nil {
		t.Fatal(err)
	}
	conversation.Receive(answer(4, domain.TypeReceipt, "c-2", "refused", "b1"))
	if entries := evidenceOf(t, conversation, 5); entries != nil {
		t.Fatalf("a refused command left evidence: %v", entries)
	}
}

func TestTheDeviceViewHoldsItsOriginAndTheLatestReadingOfEachOutput(t *testing.T) {
	t.Parallel()
	conversation := damperConversation(t)
	conversation.Receive(received(opening(2, "b1", false, "suspect", json.Number("12"))))
	view, err := conversation.Device("D1")
	if err != nil {
		t.Fatal(err)
	}
	want := domain.Origin{Board: "board-01", Environment: domain.EnvironmentSimulated}
	if view.BootID != "b1" || view.Origin != want || len(view.Outputs) != 2 || len(view.Readings) != 2 {
		t.Fatalf("view = %+v, want boot b1 on %+v with both outputs held", view, want)
	}
	if latest := view.Readings[0]; latest.Metric != "opening" || latest.Good || latest.Value != json.Number("12") || !latest.ReceivedAt.Equal(receivedAt) {
		t.Fatalf("latest opening = %+v, want the suspect 12 received at %s", latest, receivedAt)
	}
	if mode := view.Readings[1]; mode.Metric != "mode" || mode.Good {
		t.Fatalf("mode = %+v, want the birth's warming up value", mode)
	}
	if _, err := conversation.Device("D9"); !errors.Is(err, domain.ErrDeviceNotBorn) {
		t.Fatalf("Device(unborn) = %v, want ErrDeviceNotBorn", err)
	}
	before := domain.NewConversation(domain.Admission{}, 1, domain.Mapping{})
	if _, err := before.Device("D1"); !errors.Is(err, domain.ErrNoSession) {
		t.Fatalf("Device(before the handshake) = %v, want ErrNoSession", err)
	}
}
