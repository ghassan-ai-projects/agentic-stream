package app_test

import (
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/authority"
	"github.com/ghassan-ai-projects/agentic-stream/internal/plantwire/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/testsupport/plantwirefake"
)

const secondBoot = "board-01-boot-2"

func rebornWarmingUp(t *testing.T, boot string) map[string]any {
	t.Helper()
	reborn := plantwirefake.Fixture(t, ".", "DBIRTH.json")
	reborn["boot_id"] = boot
	for _, metric := range reborn["metrics"].([]any) {
		metric.(map[string]any)["value"], metric.(map[string]any)["quality"] = nil, "warming_up"
	}
	return reborn
}

func setpointData(boot string, value float64, quality string, historical bool) map[string]any {
	return map[string]any{"type": "DDATA", "lane": "data", "device": "P1", "boot_id": boot, "historical": historical,
		"metrics": []any{map[string]any{"alias": 2, "value": value, "quality": quality, "event_time_ns": birthNS + 30*second,
			"time_source": "device"}}}
}

func settled(t *testing.T, node *plantwirefake.Node) {
	t.Helper()
	node.Send(map[string]any{"type": "NSYNC", "lane": "control", "through_seq": 0, "world_time_ns": birthNS})
	node.Expect("HSYNC")
}

func assertRefusedForReconciliation(t *testing.T, host runningHost, node *plantwirefake.Node, id string) {
	t.Helper()
	if got := answered(t, dispatch(host, pumpCommand(id))); !errors.Is(got.err, authority.ErrReconciliationRequired) {
		t.Fatalf("%s = %v, want ErrReconciliationRequired", id, got.err)
	}
	assertNothingSent(t, node)
}

func assertSentOn(t *testing.T, host runningHost, node *plantwirefake.Node, id, boot string) {
	t.Helper()
	done := dispatch(host, pumpCommand(id))
	if command := node.Expect("DCMD"); command["expected_boot_id"] != boot {
		t.Fatalf("%s went to boot %v, want %s", id, command["expected_boot_id"], boot)
	}
	node.Send(receiptOnBoot(id, boot))
	succeededOn := result(id, "succeeded")
	succeededOn["boot_id"] = boot
	node.Send(succeededOn)
	if got := answered(t, done); got.err != nil || !got.effect.VerificationPending {
		t.Fatalf("%s = %+v, %v; want it sent, accepted and succeeded", id, got.effect, got.err)
	}
}

func (d deviceAuthority) resolution(t *testing.T, device string) map[string]any {
	t.Helper()
	var raw []byte
	if err := d.db.QueryRowContext(t.Context(), `SELECT resolution_evidence_json FROM device_reconciliation
		WHERE device_id = ? AND status = 'clear' AND last_resolution_status = 'succeeded'`, device).Scan(&raw); err != nil {
		t.Fatalf("no cleared resolution of %s: %v", device, err)
	}
	var evidence map[string]any
	if err := json.Unmarshal(raw, &evidence); err != nil {
		t.Fatal(err)
	}
	return evidence
}

func TestARebootReconciliationClearsOnTheNewBootsFirstGoodReadingOfItsOutput(t *testing.T) {
	t.Parallel()
	host, node, devices := commandingHostWith(t)
	succeed(t, host, node, "c-1")
	node.Send(plantwirefake.Fixture(t, ".", "DDEATH.json"))
	node.Send(rebornWarmingUp(t, secondBoot))
	settled(t, node)
	assertRefusedForReconciliation(t, host, node, "c-2")
	node.Send(setpointData(secondBoot, 60, "good", false))
	settled(t, node)
	assertSentOn(t, host, node, "c-3", secondBoot)
	evidence := devices.resolution(t, "P1")
	feedback := evidence["feedback"].(map[string]any)
	if evidence["source"] != "plantwire.reading" || evidence["boot_id"] != secondBoot || feedback["opened_by_reboot"] != true {
		t.Fatalf("resolution evidence = %v, want the new boot's reading after a reboot", evidence)
	}
}

func TestAnUnknownOutcomeClearsOnlyOnAGoodLiveReadingOfTheBootAfterIt(t *testing.T) {
	t.Parallel()
	host, node, devices := commandingHostWith(t)
	done := dispatch(host, pumpCommand("c-1"))
	node.Expect("DCMD")
	node.Send(receipt("c-1", "accepted", ""))
	node.Send(result("c-1", "unknown"))
	answered(t, done)
	assertRefusedForReconciliation(t, host, node, "c-2")
	host.clock.Advance(time.Millisecond)
	for i, sample := range []map[string]any{setpointData(firstBoot, 60, "good", true), setpointData(secondBoot, 60, "good", false),
		setpointData(firstBoot, 60, "suspect", false)} {
		node.Send(sample)
		settled(t, node)
		assertRefusedForReconciliation(t, host, node, "c-stale-"+string(rune('a'+i)))
	}
	node.Send(setpointData(firstBoot, 60, "good", false))
	settled(t, node)
	assertSentOn(t, host, node, "c-3", firstBoot)
	if feedback := devices.resolution(t, "P1")["feedback"].(map[string]any); feedback["opened_by_reboot"] != false {
		t.Fatalf("resolution feedback = %v, want the unknown outcome's opening", feedback)
	}
}

func TestTheSwitchFromASimulatedToARealSessionClearsOnTheRealDevicesFirstGoodReading(t *testing.T) {
	t.Parallel()
	devices := newDeviceAuthority(t)
	host := commandingHostOn(t, devices, hardwareHost("board-01"))
	simulated := bornNode(t, host, "s-1")
	simulated.Send(plantwirefake.Fixture(t, ".", "DBIRTH.json"))
	host.event(t)
	succeed(t, host, simulated, "c-1")
	death(simulated, 0)
	field := hardwareNode(t, host, "s-2")
	gateway := plantwirefake.Fixture(t, ".", "DBIRTH.json")
	gateway["boot_id"] = "gateway-boot-1"
	field.Send(gateway)
	settled(t, field)
	assertSentOn(t, host, field, "c-2", "gateway-boot-1")
	if evidence := devices.resolution(t, "P1"); evidence["boot_id"] != "gateway-boot-1" {
		t.Fatalf("resolution evidence = %v, want the real device's boot", evidence)
	}
}

func hardwareHost(boards ...string) domain.Settings {
	return domain.Settings{Environment: domain.EnvironmentMixed, HardwareBoards: boards, PhysicalConsent: true, Epoch: epoch}
}
