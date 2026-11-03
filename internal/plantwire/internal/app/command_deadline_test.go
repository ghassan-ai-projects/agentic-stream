package app_test

import (
	"testing"
	"time"
)

const (
	routeExpiry     = 20 * time.Second
	sinceLastRecord = 500 * time.Millisecond
)

func deadlineOfDispatchedCommand(t *testing.T, node interface{ Expect(string) map[string]any }, host runningHost) int64 {
	t.Helper()
	dispatch(host, pumpCommand("c-1"))
	return number(t, node.Expect("DCMD")["deadline_ns"])
}

func TestHostStampsADeadlineInTheTimeOfANodeWhoseClockIsFarFromTheHostsWallClock(t *testing.T) {
	t.Parallel()
	host, node := commandingHost(t)
	nodeNow := time.Unix(0, birthNS+int64(60*time.Second))
	node.Send(pumpData(firstBoot, nodeNow.UnixNano(), "good", false))
	host.event(t)
	if hostNow := host.clock.Now(); hostNow.Sub(nodeNow) < 24*time.Hour {
		t.Fatalf("the host's wall clock %s is not far from the node's %s", hostNow, nodeNow)
	}
	deadline := time.Unix(0, deadlineOfDispatchedCommand(t, node, host))
	if window := deadline.Sub(nodeNow); window != routeExpiry {
		t.Errorf("DCMD deadline is %s after the node's now, want the route's expiry %s", window, routeExpiry)
	}
}

func TestHostCarriesTheWallTimeSinceTheNodesLastRecordIntoTheDeadline(t *testing.T) {
	t.Parallel()
	host, node := commandingHost(t)
	node.Send(pumpData(firstBoot, birthNS+int64(60*time.Second), "good", false))
	host.event(t)
	host.clock.Advance(sinceLastRecord)
	deadline := time.Unix(0, deadlineOfDispatchedCommand(t, node, host))
	if want := time.Unix(0, birthNS+int64(60*time.Second)).Add(sinceLastRecord + routeExpiry); !deadline.Equal(want) {
		t.Errorf("DCMD deadline = %s, want %s", deadline, want)
	}
}

func TestHostKeepsTheWallClockDeadlineForANodeWhoseTimeIsTheWallClock(t *testing.T) {
	t.Parallel()
	host, node := commandingHost(t)
	node.Send(pumpData(firstBoot, host.clock.Now().UnixNano(), "good", false))
	host.event(t)
	deadline := time.Unix(0, deadlineOfDispatchedCommand(t, node, host))
	if want := host.clock.Now().Add(routeExpiry); !deadline.Equal(want) {
		t.Errorf("DCMD deadline = %s, want the host's wall clock plus the route's expiry %s", deadline, want)
	}
}
