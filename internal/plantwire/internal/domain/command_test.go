package domain_test

import (
	"errors"
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/plantwire/internal/domain"
)

func valveOrder(id string) domain.CommandOrder {
	return domain.CommandOrder{CommandID: id, IdempotencyKey: contractDigest, PolicyDigest: contractDigest, Device: "V1",
		Command: "set_opening", Args: map[string]any{"opening": 40.0}, ExpectedBootID: "b1", DeadlineNS: startNS}
}

func commanding(t *testing.T) *domain.Conversation {
	t.Helper()
	conversation := ingesting(t, valveInventory(t))
	conversation.Receive(deviceBirth(1, "V1", "valve", "b1", startNS))
	return conversation
}

func answer(seq uint64, recordType, id, status, boot string) domain.Arrival {
	return arrival(recordType, seq, func(a *domain.Arrival) {
		if recordType == domain.TypeReceipt {
			a.Receipt = &domain.Receipt{CommandID: id, Status: status, BootID: boot, Reason: map[bool]string{true: "local_control"}[status == "refused"]}
			return
		}
		a.Result = &domain.Result{CommandID: id, ReportedOutcome: status, BootID: boot}
	})
}

func TestConversationRefusesACommandItCannotSend(t *testing.T) {
	t.Parallel()
	unborn := ingesting(t, valveInventory(t))
	if _, err := unborn.Prepare(valveOrder("c-1")); !errors.Is(err, domain.ErrDeviceNotBorn) {
		t.Errorf("Prepare(unborn device) = %v, want ErrDeviceNotBorn", err)
	}
	before := domain.NewConversation(admissionFor(t, domain.EnvironmentSimulated, nil), 1, domain.Mapping{})
	if _, err := before.Prepare(valveOrder("c-1")); !errors.Is(err, domain.ErrNoSession) {
		t.Errorf("Prepare(before the handshake) = %v, want ErrNoSession", err)
	}
	conversation := commanding(t)
	record, err := conversation.Prepare(valveOrder("c-1"))
	if err != nil || record["type"] != domain.TypeCommand || record["expected_boot_id"] != "b1" || record["authority_epoch"] != int64(1) {
		t.Fatalf("Prepare = %v, %v; want a DCMD on boot b1 at epoch 1", record, err)
	}
	conversation.Register(valveOrder("c-1"))
	if _, err := conversation.Prepare(valveOrder("c-1")); !errors.Is(err, domain.ErrCommandInFlight) {
		t.Errorf("Prepare(the same id twice) = %v, want ErrCommandInFlight", err)
	}
	stale := valveOrder("c-2")
	stale.ExpectedBootID = "b0"
	if _, err := conversation.Prepare(stale); !errors.Is(err, domain.ErrBootChanged) {
		t.Errorf("Prepare(bound to an older boot) = %v, want ErrBootChanged", err)
	}
}

func TestCommandDigestCoversTheContentTheNodeComparesOnly(t *testing.T) {
	t.Parallel()
	first, err := domain.CommandDigest(valveOrder("c-1"))
	if err != nil {
		t.Fatal(err)
	}
	again, _ := domain.CommandDigest(valveOrder("c-2"))
	otherBoot := valveOrder("c-1")
	otherBoot.ExpectedBootID = "b2"
	moved, _ := domain.CommandDigest(otherBoot)
	if first != again || first == moved || len(first) != len("sha256:")+64 {
		t.Fatalf("digests %s, %s, %s: want the id ignored and the boot covered", first, again, moved)
	}
}

func TestConversationSettlesEachCommandFromItsAnswers(t *testing.T) {
	t.Parallel()
	conversation := commanding(t)
	for _, id := range []string{"c-ok", "c-queued", "c-refused", "c-accepted"} {
		if _, err := conversation.Prepare(valveOrder(id)); err != nil {
			t.Fatal(err)
		}
		conversation.Register(valveOrder(id))
	}
	steps := []struct {
		arrival domain.Arrival
		phase   string
		dropped string
	}{
		{answer(2, domain.TypeResult, "c-ok", "succeeded", "b1"), "", "a result for no accepted command"},
		{answer(3, domain.TypeReceipt, "c-ok", "accepted", "b1"), domain.SettlementAccepted, ""},
		{answer(4, domain.TypeResult, "c-ok", "succeeded", "b1"), domain.SettlementSucceeded, ""},
		{answer(5, domain.TypeReceipt, "c-ok", "accepted", "b1"), "", "a receipt for no command awaiting one"},
		{answer(6, domain.TypeReceipt, "c-queued", "queued", "b1"), domain.SettlementQueued, ""},
		{answer(7, domain.TypeReceipt, "c-refused", "refused", "b2"), domain.SettlementRefused, ""},
		{answer(8, domain.TypeReceipt, "c-accepted", "accepted", "b1"), domain.SettlementAccepted, ""},
	}
	for i, step := range steps {
		reaction := conversation.Receive(step.arrival)
		phase := ""
		if len(reaction.Settlements) == 1 {
			phase = reaction.Settlements[0].Phase
		}
		if phase != step.phase || reaction.Dropped != step.dropped {
			t.Errorf("step %d: settled %q dropped %q, want %q and %q", i, phase, reaction.Dropped, step.phase, step.dropped)
		}
	}
	abandoned := conversation.Abandon()
	if len(abandoned) != 2 || abandoned[0].CommandID != "c-accepted" || abandoned[0].Phase != domain.SettlementUnknown ||
		abandoned[1].Phase != domain.SettlementRefused || abandoned[1].Reason != "session_ending" || len(conversation.Abandon()) != 0 {
		t.Fatalf("Abandon = %+v, want c-accepted unknown and c-queued refused session_ending, once", abandoned)
	}
}
