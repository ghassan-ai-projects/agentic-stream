package domain_test

import (
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/plantwire/internal/domain"
)

func TestConversationAnswersTheHandshakeBarriersAndHeartbeats(t *testing.T) {
	t.Parallel()
	conversation := domain.NewConversation(admissionFor(t, domain.EnvironmentSimulated, nil), 9, domain.Mapping{})
	if conversation.Beat() != nil || conversation.Goodbye() != nil {
		t.Fatal("a conversation beats or says goodbye before its handshake")
	}
	reaction := conversation.Receive(birthArrival(t, 0, 0))
	assertReply(t, conversation.Transmit(reaction.Replies), map[string]any{"type": "STATE", "online": true, "heartbeat": uint64(0), "authority_epoch": int64(9), "seq": uint64(0), "session_id": "s-1"})
	if reaction.Verdict == nil || !reaction.Verdict.Accepted() || len(reaction.Quarantined) != 0 {
		t.Fatalf("handshake reaction = %+v, want an accepted verdict and no quarantine", reaction)
	}
	assertReply(t, conversation.Transmit(conversation.Receive(syncArrival(1)).Replies), map[string]any{"type": "HSYNC", "through_seq": uint64(1), "seq": uint64(1)})
	assertReply(t, conversation.Beat(), map[string]any{"type": "STATE", "heartbeat": uint64(1), "seq": uint64(2)})
	assertReply(t, conversation.Goodbye(), map[string]any{"type": "STATE", "online": false, "heartbeat": uint64(1)})
}

func TestConversationRefusesABirthAndEndsTheSession(t *testing.T) {
	t.Parallel()
	conversation := domain.NewConversation(admissionFor(t, domain.EnvironmentSimulated, []string{"V9"}), 1, domain.Mapping{})
	reaction := conversation.Receive(birthArrival(t, 0, 0))
	assertReply(t, conversation.Transmit(reaction.Replies), map[string]any{"type": "STATE", "online": false, "refusal": domain.RefusalInventoryMismatch})
	if reaction.End != domain.EndRefused || conversation.Beat() != nil {
		t.Fatalf("reaction = %+v, want the session ended as refused and no heartbeat", reaction)
	}
}

func TestConversationDropsWhatItCannotUse(t *testing.T) {
	t.Parallel()
	conversation := domain.NewConversation(admissionFor(t, domain.EnvironmentSimulated, nil), 1, domain.Mapping{})
	outside := syncArrival(3)
	outside.SessionID = "s-2"
	steps := []struct {
		arrival domain.Arrival
		dropped string
	}{
		{domain.Arrival{Invalid: errors.New("not json")}, "unsequenced"},
		{syncArrival(0), "before the handshake"},
		{birthArrival(t, 1, 0), ""},
		{syncArrival(1), "duplicate"},
		{domain.Arrival{Envelope: domain.Envelope{Type: "DDATA", Seq: 2}, Sequenced: true, Invalid: errors.New("bad")}, "invalid: bad"},
		{outside, "outside the session"},
		{deathArrival(4, 0), ""},
	}
	for i, step := range steps {
		if got := conversation.Receive(step.arrival).Dropped; got != step.dropped {
			t.Errorf("step %d dropped %q, want %q", i, got, step.dropped)
		}
	}
}

func TestConversationAsksOneRebirthPerGapUntilTheNodeBirthsAgain(t *testing.T) {
	t.Parallel()
	conversation := domain.NewConversation(admissionFor(t, domain.EnvironmentSimulated, nil), 1, domain.Mapping{})
	conversation.Receive(birthArrival(t, 0, 0))
	first := conversation.Receive(syncArrival(2))
	second := conversation.Receive(syncArrival(4))
	if !first.Rebirth || replyTypes(first) != "NCMD,HSYNC" || second.Rebirth || replyTypes(second) != "HSYNC" {
		t.Fatalf("gaps gave %s then %s, want one NCMD rebirth before the first HSYNC only", replyTypes(first), replyTypes(second))
	}
	rebirth := conversation.Receive(birthArrival(t, 5, 1))
	if rebirth.Verdict == nil || !rebirth.Verdict.Accepted() || len(rebirth.Replies) != 0 {
		t.Fatalf("a rebirth gave %+v, want an accepted verdict and no new handshake", rebirth)
	}
	if again := conversation.Receive(syncArrival(7)); !again.Rebirth {
		t.Fatal("a gap after the rebirth asked no new rebirth")
	}
}

func TestConversationEndsOnTheDeathOfTheCurrentBirthOnly(t *testing.T) {
	t.Parallel()
	conversation := domain.NewConversation(admissionFor(t, domain.EnvironmentSimulated, nil), 1, domain.Mapping{})
	conversation.Receive(birthArrival(t, 0, 2))
	if stale := conversation.Receive(deathArrival(1, 1)); stale.End != "" || stale.Dropped != "a node death of an older birth" {
		t.Fatalf("an older death gave %+v, want it dropped", stale)
	}
	if current := conversation.Receive(deathArrival(2, 2)); current.End != domain.EndNodeDeath {
		t.Fatalf("the current death gave %+v, want the session ended", current)
	}
}

func TestConversationReportsTheKindsItQuarantines(t *testing.T) {
	t.Parallel()
	conversation := domain.NewConversation(admissionFor(t, domain.EnvironmentSimulated, nil), 1, domain.Mapping{})
	arrival := birthArrival(t, 0, 0)
	arrival.Birth.Kinds = append(arrival.Birth.Kinds, announced(t, domain.Kind{"kind": "pump", "metrics": []any{}, "commands": []any{}}))
	if reaction := conversation.Receive(arrival); !slices.Equal(reaction.Quarantined, []string{"pump"}) {
		t.Fatalf("Quarantined = %v, want [pump]", reaction.Quarantined)
	}
}

func TestConversationRefusesARebirthThatContradictsTheAdmission(t *testing.T) {
	t.Parallel()
	conversation := domain.NewConversation(admissionFor(t, domain.EnvironmentSimulated, nil), 1, domain.Mapping{})
	conversation.Receive(birthArrival(t, 0, 0))
	rebirth := birthArrival(t, 1, 1)
	rebirth.Birth.Environment = domain.EnvironmentHardware
	rebirth.Birth.Boards = []domain.Board{{Board: "board-01", Environment: domain.EnvironmentHardware, Members: []string{"V1"}}}
	got := conversation.Receive(rebirth)
	if got.Verdict == nil || got.Verdict.Accepted() || got.End != domain.EndRefused {
		t.Fatalf("a rebirth that contradicts the admission gave %+v, want the session refused", got)
	}
	if len(got.Replies) != 1 || got.Replies[0].Fields["refusal"] != domain.RefusalEnvironmentMismatch {
		t.Fatalf("the refusal replies = %v, want a STATE naming the environment mismatch", got.Replies)
	}
}

func TestConversationDropsARebirthThatDoesNotAdvanceTheBirthSequence(t *testing.T) {
	t.Parallel()
	conversation := domain.NewConversation(admissionFor(t, domain.EnvironmentSimulated, nil), 1, domain.Mapping{})
	conversation.Receive(birthArrival(t, 0, 3))
	rebirth := conversation.Receive(birthArrival(t, 1, 3))
	if rebirth.Dropped != "a birth that does not advance bd_seq" || rebirth.End != "" {
		t.Fatalf("a birth that does not advance bd_seq gave %+v, want it dropped and the session alive", rebirth)
	}
}

func TestConversationReportsTheOriginOfADeviceThatMovedToAnotherBoard(t *testing.T) {
	t.Parallel()
	inventory := valveInventory(t)
	mapping, err := domain.NewMapping(valveSite(), inventory)
	if err != nil {
		t.Fatal(err)
	}
	admission := domain.Admission{Environment: domain.EnvironmentMixed, ContractDigest: contractDigest, Inventory: inventory}
	conversation := domain.NewConversation(admission, 1, mapping)
	conversation.Receive(birthArrival(t, 0, 0))
	conversation.Receive(deviceBirthOnBoard(1, "V1", "valve", "b1", "board-01", startNS))
	origin, err := conversation.OriginOf("V1")
	if err != nil || origin.Board != "board-01" || origin.Environment != domain.EnvironmentSimulated {
		t.Fatalf("OriginOf = %+v, %v, want the device on the simulated board-01", origin, err)
	}
	moved := birthArrival(t, 2, 1)
	moved.Birth.Environment = domain.EnvironmentMixed
	moved.Birth.Boards = []domain.Board{
		{Board: "board-01", Environment: domain.EnvironmentSimulated, Members: []string{}},
		{Board: "hw-9", Environment: domain.EnvironmentHardware, Members: []string{"V1"}},
	}
	conversation.Receive(moved)
	conversation.Receive(deviceBirthOnBoard(3, "V1", "valve", "b2", "hw-9", startNS))
	origin, err = conversation.OriginOf("V1")
	if err != nil || origin.Board != "hw-9" || origin.Environment != domain.EnvironmentHardware {
		t.Fatalf("OriginOf after the rebirth = %+v, %v, want the device on the hardware board hw-9", origin, err)
	}
}

func deviceBirthOnBoard(seq uint64, device, kind, boot, board string, atNS int64) domain.Arrival {
	birth := domain.DeviceBirth{Device: device, Kind: kind, BootID: boot, Board: board, BirthTimeNS: atNS,
		Metrics: []domain.BirthMetric{{Name: "opening", Alias: 1, Quality: "good"}}}
	return arrival(domain.TypeDeviceBirth, seq, func(a *domain.Arrival) { a.DeviceBirth = &birth })
}

func birthArrival(t *testing.T, seq, bdSeq uint64) domain.Arrival {
	t.Helper()
	birth := birthOf(t, func(b *domain.Birth) { b.Seq, b.BDSeq = seq, bdSeq })
	return domain.Arrival{Envelope: birth.Envelope, Sequenced: true, Birth: &birth}
}

func syncArrival(seq uint64) domain.Arrival {
	return domain.Arrival{Envelope: domain.Envelope{Type: domain.TypeSync, ProtocolVersion: 1, SessionID: "s-1", Seq: seq}, Sequenced: true,
		Sync: &domain.Sync{WorldTimeNS: 1}}
}

func deathArrival(seq, bdSeq uint64) domain.Arrival {
	return domain.Arrival{Envelope: domain.Envelope{Type: domain.TypeNodeDeath, ProtocolVersion: 1, SessionID: "s-1", Seq: seq},
		Sequenced: true, Death: &domain.NodeDeath{BDSeq: bdSeq, Reason: "shutdown"}}
}

func assertReply(t *testing.T, replies []map[string]any, want map[string]any) {
	t.Helper()
	if len(replies) != 1 {
		t.Fatalf("replies = %v, want one %v", replies, want["type"])
	}
	for member, value := range want {
		if replies[0][member] != value {
			t.Errorf("%v %s = %#v, want %#v", want["type"], member, replies[0][member], value)
		}
	}
}

func replyTypes(reaction domain.Reaction) string {
	var types []string
	for _, reply := range reaction.Replies {
		types = append(types, reply.Type)
	}
	return strings.Join(types, ",")
}

func TestConversationNumbersAReplyWhenItIsTransmittedNotWhenItIsProduced(t *testing.T) {
	t.Parallel()
	conversation := domain.NewConversation(admissionFor(t, domain.EnvironmentSimulated, nil), 9, domain.Mapping{})
	conversation.Transmit(conversation.Receive(birthArrival(t, 0, 0)).Replies)
	held := conversation.Receive(syncArrival(1)).Replies
	beat := conversation.Beat()
	answered := conversation.Transmit(held)
	if beat[0]["seq"] != uint64(1) || answered[0]["seq"] != uint64(2) || answered[0]["type"] != domain.TypeHostSync {
		t.Fatalf("heartbeat %v then held answer %v, want seq 1 then seq 2", beat, answered)
	}
}
