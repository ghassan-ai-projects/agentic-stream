package app_test

import (
	"encoding/json"
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/actionport"
	"github.com/ghassan-ai-projects/agentic-stream/internal/authority"
	"github.com/ghassan-ai-projects/agentic-stream/internal/plantwire/internal/app"
	"github.com/ghassan-ai-projects/agentic-stream/internal/testsupport/plantwirefake"
)

func setpointCommand(id string) actionport.Command {
	command := pumpCommand(id)
	command.EffectorRoute, command.Payload = "hold_pump_setpoint", map[string]any{"setpoint": "eighty"}
	return command
}

func succeedWith(t *testing.T, host runningHost, node *plantwirefake.Node, command actionport.Command) {
	t.Helper()
	done := dispatch(host, command)
	node.Expect("DCMD")
	node.Send(receipt(command.CommandID, "accepted", ""))
	node.Send(result(command.CommandID, "succeeded"))
	if got := answered(t, done); got.err != nil || !got.effect.VerificationPending {
		t.Fatalf("%s = %+v, %v; want it pending verification", command.CommandID, got.effect, got.err)
	}
}

func verify(t *testing.T, host runningHost, command actionport.Command) (string, map[string]any) {
	t.Helper()
	status, evidence, err := host.host.VerifyDeviceCommand(t.Context(), command)
	if err != nil {
		t.Fatalf("VerifyDeviceCommand(%s) = %v", command.CommandID, err)
	}
	return status, evidence
}

func TestASucceededCommandIsVerifiedByTheDevicesNextGoodReadingOfItsOutput(t *testing.T) {
	t.Parallel()
	host, node := commandingHost(t)
	command := setpointCommand("c-1")
	succeedWith(t, host, node, command)
	node.Send(setpointData(firstBoot, 70, "good", false))
	node.Send(setpointData(firstBoot, 80, "good", false))
	node.Send(map[string]any{"type": "NSYNC", "lane": "control", "through_seq": 0, "world_time_ns": birthNS})
	entries := node.Expect("HSYNC")["command_evidence"].([]any)
	if entry := entries[0].(map[string]any); len(entries) != 1 || entry["outcome"] != "observed" || entry["metric"] != "speed_setpoint" ||
		entry["observed_value"] != json.Number("80") {
		t.Fatalf("HSYNC command_evidence = %v, want speed_setpoint observed at 80", entries)
	}
	status, evidence := verify(t, host, command)
	if status != actionport.CommandSucceeded {
		t.Fatalf("VerifyDeviceCommand = %q, want succeeded", status)
	}
	parsed, err := authority.SealReconciliationEvidence(evidence["source"].(string), "P1", evidence["state"].(map[string]any), evidence["feedback"].(map[string]any))
	if err != nil || parsed.EvidenceDigest != evidence["evidence_digest"] || evidence["evidence_type"] != "device_state_feedback" || evidence["boot_id"] != firstBoot {
		t.Fatalf("evidence = %v, %v; want sealed device-state feedback of %s", evidence, err, firstBoot)
	}
}

func TestACommandStaysUnverifiedWithoutAMatchingReadingInTheWindow(t *testing.T) {
	t.Parallel()
	host, node := commandingHost(t)
	command := setpointCommand("c-1")
	succeedWith(t, host, node, command)
	node.Send(setpointData(firstBoot, 70, "good", false))
	node.Send(setpointData(firstBoot, 80, "good", true))
	settled(t, node)
	host.clock.Advance(app.VerificationWindow)
	if status, evidence := verify(t, host, command); status != "" || evidence != nil {
		t.Fatalf("VerifyDeviceCommand = %q, %v; want no final status", status, evidence)
	}
}

func TestACommandWithNothingToReadBackOrNotSentHereIsNotVerified(t *testing.T) {
	t.Parallel()
	host, node := commandingHost(t)
	succeed(t, host, node, "c-1")
	for _, command := range []actionport.Command{pumpCommand("c-1"), pumpCommand("c-1"), setpointCommand("c-never-sent")} {
		if status, _ := verify(t, host, command); status != "" {
			t.Fatalf("VerifyDeviceCommand(%s) = %q, want no final status", command.CommandID, status)
		}
	}
}

func TestAWaitingVerificationEndsWithItsSession(t *testing.T) {
	t.Parallel()
	host, node := commandingHost(t)
	command := setpointCommand("c-1")
	succeedWith(t, host, node, command)
	node.Close()
	if status, _ := verify(t, host, command); status != "" {
		t.Fatalf("VerifyDeviceCommand after the session ended = %q, want no final status", status)
	}
}
