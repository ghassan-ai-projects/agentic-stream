package domain_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/plantwire/internal/domain"
)

const startNS = int64(1767225600000000000)

type step struct {
	arrival domain.Arrival
	events  []string
	dropped string
}

func ingesting(t *testing.T, inventory domain.Inventory) *domain.Conversation {
	t.Helper()
	mapping, err := domain.NewMapping(valveSite(), valveInventory(t))
	if err != nil {
		t.Fatal(err)
	}
	admission := domain.Admission{Environment: domain.EnvironmentSimulated, ContractDigest: contractDigest, Inventory: inventory}
	conversation := domain.NewConversation(admission, 1, mapping)
	conversation.Receive(birthArrival(t, 0, 0))
	return conversation
}

func play(t *testing.T, conversation *domain.Conversation, steps []step) {
	t.Helper()
	for i, step := range steps {
		reaction := conversation.Receive(step.arrival)
		var events []string
		for _, event := range reaction.Events {
			events = append(events, event.Type+"@"+event.EventTime.Format("05"))
		}
		if reaction.Dropped != step.dropped || len(events) != len(step.events) || len(events) > 0 && events[0] != step.events[0] ||
			len(events) > 1 && events[1] != step.events[1] {
			t.Errorf("step %d (%s): events %v dropped %q, want %v dropped %q", i, step.arrival.Type, events, reaction.Dropped, step.events, step.dropped)
		}
	}
}

func TestConversationDropsDeviceRecordsItCannotTrust(t *testing.T) {
	t.Parallel()
	conversation := ingesting(t, valveInventory(t))
	play(t, conversation, []step{
		{arrival: deviceBirth(1, "P1", "pump", "b1", startNS), dropped: "a device of a kind the node birth did not commission"},
		{arrival: deviceBirth(2, "V1", "Valve", "b1", startNS), dropped: "a device born as another kind than the site file names"},
		{arrival: data(3, "V1", "b1", false, sample(startNS, "good")), dropped: "data of a device that is not born on this boot"},
		{arrival: deviceBirth(4, "V1", "valve", "b1", startNS), events: []string{"line.heartbeat.observed@00"}},
		{arrival: data(5, "V1", "b0", false, sample(startNS, "good")), dropped: "data of a device that is not born on this boot"},
		{arrival: deviceDeath(6, "V1", "b0"), dropped: "the death of a device that is not born on this boot"},
		{arrival: data(7, "V1", "b1", true, sample(startNS+time.Hour.Nanoseconds(), "good")), dropped: "historical data is not ingested live"},
		{arrival: data(8, "V1", "b1", false, domain.Sample{Alias: 7, Quality: "good", EventTimeNS: startNS})},
		{arrival: deviceBirth(9, "V7", "valve", "b1", startNS)},
	})
	if tally := conversation.Tally(); tally != (domain.Tally{Events: 1, Historical: 1, Dropped: 5}) {
		t.Fatalf("Tally = %+v, want 1 event, 1 historical record and 5 dropped records", tally)
	}
}

func TestConversationIgnoresTheDevicesOfAQuarantinedKind(t *testing.T) {
	t.Parallel()
	drifted := valveKind()
	drifted["metrics"] = append(drifted["metrics"].([]any), map[string]any{"name": "torque", "data_type": "number"})
	inventory, err := domain.Commission([]domain.Kind{drifted}, nil)
	if err != nil {
		t.Fatal(err)
	}
	play(t, ingesting(t, inventory), []step{
		{arrival: deviceBirth(1, "V1", "valve", "b1", startNS), dropped: "a device of a kind the node birth did not commission"},
	})
}

func TestHeartbeatFollowsEventTimeWhileTheDeviceIsAliveAndFresh(t *testing.T) {
	t.Parallel()
	play(t, ingesting(t, valveInventory(t)), []step{
		{arrival: deviceBirth(1, "V1", "valve", "b1", startNS), events: []string{"line.heartbeat.observed@00"}},
		{arrival: syncAt(2, startNS+5*seconds)},
		{arrival: syncAt(3, startNS+10*seconds)},
		{arrival: data(4, "V1", "b1", false, sample(startNS+12*seconds, "good")), events: []string{"line.opening.observed@12", "line.heartbeat.observed@12"}},
		{arrival: syncAt(5, startNS+21*seconds)},
		{arrival: data(6, "V1", "b1", false, sample(startNS+30*seconds, "suspect")), events: []string{"line.opening.observed@30"}},
		{arrival: data(7, "V1", "b1", false, sample(startNS+35*seconds, "good")), events: []string{"line.opening.observed@35", "line.heartbeat.observed@35"}},
		{arrival: syncAt(8, startNS+41*seconds)},
		{arrival: data(9, "V1", "b1", false, sample(startNS+44*seconds, "good")), events: []string{"line.opening.observed@44"}},
		{arrival: syncAt(10, startNS+45*seconds), events: []string{"line.heartbeat.observed@45"}},
		{arrival: deviceDeath(11, "V1", "b1")},
		{arrival: syncAt(12, startNS+55*seconds)},
		{arrival: data(13, "V1", "b1", false, sample(startNS+56*seconds, "good")), dropped: "data of a device that is not born on this boot"},
	})
}

func TestARebirthForgetsTheBornDevices(t *testing.T) {
	t.Parallel()
	play(t, ingesting(t, valveInventory(t)), []step{
		{arrival: deviceBirth(1, "V1", "valve", "b1", startNS), events: []string{"line.heartbeat.observed@00"}},
		{arrival: birthArrival(t, 2, 1)},
		{arrival: data(3, "V1", "b1", false, sample(startNS+time.Second.Nanoseconds(), "good")), dropped: "data of a device that is not born on this boot"},
	})
}

const seconds = int64(time.Second)

func deviceBirth(seq uint64, device, kind, boot string, atNS int64) domain.Arrival {
	birth := domain.DeviceBirth{Device: device, Kind: kind, BootID: boot, BirthTimeNS: atNS,
		Metrics: []domain.BirthMetric{{Name: "opening", Alias: 1, Quality: "good"}}}
	return arrival(domain.TypeDeviceBirth, seq, func(a *domain.Arrival) { a.DeviceBirth = &birth })
}

func data(seq uint64, device, boot string, historical bool, samples ...domain.Sample) domain.Arrival {
	body := domain.DeviceData{Device: device, BootID: boot, Historical: historical, Metrics: samples}
	return arrival(domain.TypeDeviceData, seq, func(a *domain.Arrival) { a.DeviceData = &body })
}

func deviceDeath(seq uint64, device, boot string) domain.Arrival {
	return arrival(domain.TypeDeviceDeath, seq, func(a *domain.Arrival) { a.DeviceDeath = &domain.DeviceDeath{Device: device, BootID: boot} })
}

func syncAt(seq uint64, atNS int64) domain.Arrival {
	return arrival(domain.TypeSync, seq, func(a *domain.Arrival) { a.Sync = &domain.Sync{WorldTimeNS: atNS} })
}

func sample(atNS int64, quality string) domain.Sample {
	return domain.Sample{Alias: 1, Value: json.Number("12.5"), Quality: quality, EventTimeNS: atNS}
}

func arrival(recordType string, seq uint64, body func(*domain.Arrival)) domain.Arrival {
	a := domain.Arrival{Envelope: domain.Envelope{Type: recordType, ProtocolVersion: 1, SessionID: "s-1", Seq: seq}, Sequenced: true}
	body(&a)
	return a
}
