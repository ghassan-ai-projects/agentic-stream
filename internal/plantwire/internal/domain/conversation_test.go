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
	conversation := domain.NewConversation(admissionFor(t, domain.EnvironmentSimulated, nil), 9)
	if conversation.Beat() != nil || conversation.Goodbye() != nil {
		t.Fatal("a conversation beats or says goodbye before its handshake")
	}
	reaction := conversation.Receive(birthArrival(t, 0, 0))
	assertReply(t, reaction, map[string]any{"type": "STATE", "online": true, "heartbeat": uint64(0), "authority_epoch": int64(9), "seq": uint64(0), "session_id": "s-1"})
	if reaction.Verdict == nil || !reaction.Verdict.Accepted() || len(reaction.Quarantined) != 0 {
		t.Fatalf("handshake reaction = %+v, want an accepted verdict and no quarantine", reaction)
	}
	assertReply(t, conversation.Receive(syncArrival(1)), map[string]any{"type": "HSYNC", "through_seq": uint64(1), "seq": uint64(1)})
	assertReply(t, domain.Reaction{Replies: conversation.Beat()}, map[string]any{"type": "STATE", "heartbeat": uint64(1), "seq": uint64(2)})
	assertReply(t, domain.Reaction{Replies: conversation.Goodbye()}, map[string]any{"type": "STATE", "online": false, "heartbeat": uint64(1)})
}

func TestConversationRefusesABirthAndEndsTheSession(t *testing.T) {
	t.Parallel()
	conversation := domain.NewConversation(admissionFor(t, domain.EnvironmentSimulated, []string{"V9"}), 1)
	reaction := conversation.Receive(birthArrival(t, 0, 0))
	assertReply(t, reaction, map[string]any{"type": "STATE", "online": false, "refusal": domain.RefusalInventoryMismatch})
	if reaction.End != domain.EndRefused || conversation.Beat() != nil {
		t.Fatalf("reaction = %+v, want the session ended as refused and no heartbeat", reaction)
	}
}

func TestConversationDropsWhatItCannotUse(t *testing.T) {
	t.Parallel()
	conversation := domain.NewConversation(admissionFor(t, domain.EnvironmentSimulated, nil), 1)
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
	conversation := domain.NewConversation(admissionFor(t, domain.EnvironmentSimulated, nil), 1)
	conversation.Receive(birthArrival(t, 0, 0))
	first := conversation.Receive(syncArrival(2))
	second := conversation.Receive(syncArrival(4))
	if !first.Rebirth || replyTypes(first) != "NCMD,HSYNC" || second.Rebirth || replyTypes(second) != "HSYNC" {
		t.Fatalf("gaps gave %s then %s, want one NCMD rebirth before the first HSYNC only", replyTypes(first), replyTypes(second))
	}
	rebirth := conversation.Receive(birthArrival(t, 5, 1))
	if rebirth.Verdict != nil || len(rebirth.Replies) != 0 {
		t.Fatalf("a rebirth gave %+v, want no new handshake", rebirth)
	}
	if again := conversation.Receive(syncArrival(7)); !again.Rebirth {
		t.Fatal("a gap after the rebirth asked no new rebirth")
	}
}

func TestConversationEndsOnTheDeathOfTheCurrentBirthOnly(t *testing.T) {
	t.Parallel()
	conversation := domain.NewConversation(admissionFor(t, domain.EnvironmentSimulated, nil), 1)
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
	conversation := domain.NewConversation(admissionFor(t, domain.EnvironmentSimulated, nil), 1)
	arrival := birthArrival(t, 0, 0)
	arrival.Birth.Kinds = append(arrival.Birth.Kinds, announced(t, domain.Kind{"kind": "pump", "metrics": []any{}, "commands": []any{}}))
	if reaction := conversation.Receive(arrival); !slices.Equal(reaction.Quarantined, []string{"pump"}) {
		t.Fatalf("Quarantined = %v, want [pump]", reaction.Quarantined)
	}
}

func birthArrival(t *testing.T, seq, bdSeq uint64) domain.Arrival {
	t.Helper()
	birth := birthOf(t, func(b *domain.Birth) { b.Seq, b.BDSeq = seq, bdSeq })
	return domain.Arrival{Envelope: birth.Envelope, Sequenced: true, Birth: &birth}
}

func syncArrival(seq uint64) domain.Arrival {
	return domain.Arrival{Envelope: domain.Envelope{Type: domain.TypeSync, ProtocolVersion: 1, SessionID: "s-1", Seq: seq}, Sequenced: true}
}

func deathArrival(seq, bdSeq uint64) domain.Arrival {
	return domain.Arrival{Envelope: domain.Envelope{Type: domain.TypeNodeDeath, ProtocolVersion: 1, SessionID: "s-1", Seq: seq},
		Sequenced: true, Death: &domain.NodeDeath{BDSeq: bdSeq, Reason: "shutdown"}}
}

func assertReply(t *testing.T, reaction domain.Reaction, want map[string]any) {
	t.Helper()
	if len(reaction.Replies) != 1 {
		t.Fatalf("replies = %v, want one %v", reaction.Replies, want["type"])
	}
	for member, value := range want {
		if reaction.Replies[0][member] != value {
			t.Errorf("%v %s = %#v, want %#v", want["type"], member, reaction.Replies[0][member], value)
		}
	}
}

func replyTypes(reaction domain.Reaction) string {
	var types []string
	for _, reply := range reaction.Replies {
		types = append(types, reply["type"].(string))
	}
	return strings.Join(types, ",")
}
