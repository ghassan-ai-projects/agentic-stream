package app_test

import (
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/plantwire/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/testsupport/plantwirefake"
)

func TestHostAnswersEveryBarrierAtOnceAndBeatsEveryInterval(t *testing.T) {
	t.Parallel()
	host := startHost(t, simulatedHost)
	node := bornNode(t, host, "s-1")
	barrier(node)
	if answer := node.Expect("HSYNC"); number(t, answer["through_seq"]) != 1 || number(t, answer["seq"]) != 1 {
		t.Errorf("HSYNC = %v, want through_seq 1 at host seq 1", answer)
	}
	for beat := int64(1); beat <= 2; beat++ {
		host.clock.Advance(domain.HeartbeatInterval)
		if state := node.Expect("STATE"); number(t, state["heartbeat"]) != beat || state["online"] != true {
			t.Errorf("heartbeat %d STATE = %v", beat, state)
		}
	}
}

func TestHostRefusesABirthItCannotAdmitAndEndsTheSession(t *testing.T) {
	t.Parallel()
	host := startHost(t, domain.Settings{Environment: domain.EnvironmentHardware, PhysicalConsent: true, Epoch: epoch})
	node := host.node(t, "s-1")
	node.Send(map[string]any{"type": "NSYNC", "lane": "control", "through_seq": 0, "world_time_ns": 0})
	node.SendLine([]byte("not a record"))
	node.Send(fixtureBirth(t))
	if state := node.Expect("STATE"); state["online"] != false || state["refusal"] != domain.RefusalEnvironmentMismatch {
		t.Fatalf("STATE = %v, want a refusal environment_mismatch", state)
	}
	if !node.Closed() {
		t.Fatal("the host kept a refused session open")
	}
}

func TestHostAsksOneRebirthUntilTheNodeBirthsAgain(t *testing.T) {
	t.Parallel()
	host := startHost(t, simulatedHost)
	node := bornNode(t, host, "s-1")
	node.Skip()
	barrier(node)
	node.Expect("NCMD")
	node.Expect("HSYNC")
	node.Skip()
	barrier(node)
	node.Expect("HSYNC")
	rebirth := fixtureBirth(t)
	rebirth["bd_seq"] = 1
	node.Send(rebirth)
	node.Skip()
	barrier(node)
	if command := node.Expect("NCMD"); command["command"] != "rebirth" {
		t.Errorf("NCMD = %v, want a rebirth after the node answered the last one", command)
	}
}

func TestHostAcceptsTheNextSessionAfterTheLastOneEnds(t *testing.T) {
	t.Parallel()
	host := startHost(t, simulatedHost)
	first := bornNode(t, host, "s-1")
	death(first, 0)
	if !first.Closed() {
		t.Fatal("the host kept the session open after the node died")
	}
	second := bornNode(t, host, "s-2")
	second.Close()
	third := bornNode(t, host, "s-3")
	barrier(third)
	third.Expect("HSYNC")
}

func TestHostServesASecondConnectionOnlyAfterTheLiveSessionEnds(t *testing.T) {
	t.Parallel()
	host := startHost(t, simulatedHost)
	live := bornNode(t, host, "s-1")
	waiting := host.node(t, "s-2")
	waiting.Send(fixtureBirth(t))
	barrier(live)
	live.Expect("HSYNC")
	death(live, 0)
	if state := waiting.Expect("STATE"); state["online"] != true || state["session_id"] != "s-2" {
		t.Fatalf("the waiting node got %v, want its handshake once the live session ended", state)
	}
}

func TestHostSaysGoodbyeWhenItStops(t *testing.T) {
	t.Parallel()
	host := startHost(t, simulatedHost)
	node := bornNode(t, host, "s-1")
	host.cancel()
	if state := node.Expect("STATE"); state["online"] != false {
		t.Fatalf("STATE = %v, want the goodbye online false", state)
	}
	if !node.Closed() {
		t.Fatal("the session stayed open after the goodbye")
	}
}

func TestCloseStopsTheHostWithoutAnError(t *testing.T) {
	t.Parallel()
	host := startHost(t, simulatedHost)
	if err := host.host.Close(); err != nil {
		t.Fatalf("Close = %v", err)
	}
	host.stop(t)
}

func fixtureBirth(t *testing.T) map[string]any {
	t.Helper()
	return plantwirefake.Fixture(t, ".", "NBIRTH.json")
}
