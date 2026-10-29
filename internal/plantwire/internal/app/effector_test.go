package app_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/actionport"
	"github.com/ghassan-ai-projects/agentic-stream/internal/canonicaljson"
	"github.com/ghassan-ai-projects/agentic-stream/internal/kernel"
	"github.com/ghassan-ai-projects/agentic-stream/internal/plantwire/internal/app"
	"github.com/ghassan-ai-projects/agentic-stream/internal/plantwire/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/testsupport/plantwirefake"
)

const pumpCatalog = `{"protocol_version": 1, "routes": {"set_pump_speed": {"operation": "set_speed", "target": "P1",
 "target_bindings": {"station-1": "P1"}, "selector_field": "speed", "expires_after_ms": 20000,
 "presets": {"half": {"pct": 50, "lease_ms": 5000}}, "bounds": {"pct": {"min": 0, "max": 100}}},
 "hold_pump_setpoint": {"operation": "set_speed", "target": "P1", "target_bindings": {"station-1": "P1"}, "selector_field": "setpoint",
 "expires_after_ms": 20000, "presets": {"eighty": {"speed_setpoint": 80, "lease_ms": 5000}}}}}`

const (
	idempotencyKey = "sha256:1111111111111111111111111111111111111111111111111111111111111111"
	policyDigest   = "sha256:2222222222222222222222222222222222222222222222222222222222222222"
)

type dispatched struct {
	effect actionport.Effect
	err    error
}

func commandingHost(t *testing.T) (runningHost, *plantwirefake.Node) {
	t.Helper()
	host, node, _ := commandingHostWith(t)
	return host, node
}

func commandingHostWith(t *testing.T) (runningHost, *plantwirefake.Node, deviceAuthority) {
	t.Helper()
	devices := newDeviceAuthority(t)
	host := pumpHost(t, commandingConfig(t, devices), nil)
	node := bornNode(t, host, "s-1")
	node.Send(plantwirefake.Fixture(t, ".", "DBIRTH.json"))
	host.event(t)
	return host, node, devices
}

func pumpCommand(id string) actionport.Command {
	return actionport.Command{CommandID: id, IntentID: "intent-" + id, EffectorRoute: "set_pump_speed", NormalizedTarget: "station-1",
		IdempotencyKey: idempotencyKey, PolicyDigest: policyDigest, Payload: map[string]any{"speed": "half"}}
}

func dispatch(host runningHost, command actionport.Command) <-chan dispatched {
	done := make(chan dispatched, 1)
	allow := actionport.Authorization{Check: func(context.Context) error { return nil }}
	go func() {
		effect, err := host.host.DispatchAuthorized(context.Background(), command, allow)
		done <- dispatched{effect: effect, err: err}
	}()
	return done
}

func answered(t *testing.T, done <-chan dispatched) dispatched {
	t.Helper()
	select {
	case result := <-done:
		return result
	case <-time.After(5 * time.Second):
		t.Fatal("the dispatch never returned")
		return dispatched{}
	}
}

func receipt(id, status, reason string) map[string]any {
	record := map[string]any{"type": "DRECEIPT", "lane": "control", "command_id": id, "status": status, "boot_id": firstBoot}
	if reason != "" {
		record["reason"] = reason
	}
	return record
}

func result(id, outcome string) map[string]any {
	return map[string]any{"type": "DRESULT", "lane": "control", "command_id": id, "reported_outcome": outcome, "boot_id": firstBoot}
}

func TestHostSendsTheGovernedCommandAsOneDCMD(t *testing.T) {
	t.Parallel()
	host, node := commandingHost(t)
	nodeDeadline := time.Unix(0, birthNS).Add(20 * time.Second)
	done := dispatch(host, pumpCommand("c-1"))
	command := node.Expect("DCMD")
	want := map[string]any{"command_id": "c-1", "idempotency_key": idempotencyKey, "device": "P1", "command": "set_speed",
		"expected_boot_id": firstBoot, "policy_digest": policyDigest, "fallback_on_lease_loss": "safe_off", "lane": "control", "session_id": "s-1"}
	for member, value := range want {
		if command[member] != value {
			t.Errorf("DCMD %s = %v, want %v", member, command[member], value)
		}
	}
	args := command["args"].(map[string]any)
	if number(t, command["deadline_ns"]) != nodeDeadline.UnixNano() || number(t, command["authority_epoch"]) != epoch ||
		number(t, args["pct"]) != 50 || number(t, args["lease_ms"]) != 5000 || number(t, command["not_before_ns"]) != 0 {
		t.Errorf("DCMD = %v, want the deadline 20 s after the node's time %s, epoch %d and the preset's args", command, nodeDeadline, epoch)
	}
	node.Send(receipt("c-1", "accepted", ""))
	node.Send(result("c-1", "succeeded"))
	got := answered(t, done)
	if got.err != nil || !got.effect.VerificationPending {
		t.Fatalf("an accepted and succeeded command gave %+v, %v; want it pending verification", got.effect, got.err)
	}
	if answers := got.effect.ProviderResult["plant_wire"].(map[string]any)["answers"].([]any); len(answers) != 2 {
		t.Fatalf("provider result answers = %v, want the receipt and the result", answers)
	}
	if _, err := canonicaljson.Marshal(got.effect.ProviderResult); err != nil {
		t.Fatalf("the provider result has no canonical form, so the outcome ledger cannot digest it: %v", err)
	}
	if deadline := got.effect.ProviderResult["plant_wire"].(map[string]any)["deadline"]; deadline != kernel.FormatTime(nodeDeadline) {
		t.Fatalf("provider result deadline = %v, want the deadline as durable time text", deadline)
	}
}

func TestHostMapsEveryTerminalAnswerToTheOutcomeLedger(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		answers []map[string]any
		want    func(error) bool
		reason  string
	}{
		{"refused for local control", []map[string]any{receipt("c-1", "refused", "local_control")}, is(app.ErrCommandRefused), "local_control"},
		{"refused for a stale epoch", []map[string]any{receipt("c-1", "refused", "stale_epoch")}, is(app.ErrCommandRefused), "stale_epoch"},
		{"refused by the policy allow-list", []map[string]any{receipt("c-1", "refused", "policy_not_allowed")}, is(app.ErrCommandRefused), "policy_not_allowed"},
		{"refused on the wrong boot", []map[string]any{receipt("c-1", "refused", "wrong_boot")}, is(app.ErrCommandRefused), "wrong_boot"},
		{"queued then expired", []map[string]any{receipt("c-1", "queued", ""), receipt("c-1", "refused", "expired")}, is(app.ErrCommandRefused), "expired"},
		{"accepted then failed", []map[string]any{receipt("c-1", "accepted", ""), result("c-1", "failed")}, is(app.ErrCommandFailed), "the device reported failed"},
		{"accepted then unknown", []map[string]any{receipt("c-1", "accepted", ""), result("c-1", "unknown")}, actionport.IsUnknownOutcome, "the device reported unknown"},
		{"accepted on another boot", []map[string]any{receiptOnBoot("c-1", "board-01-boot-9")}, actionport.IsUnknownOutcome, "another boot"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			host, node := commandingHost(t)
			done := dispatch(host, pumpCommand("c-1"))
			node.Expect("DCMD")
			for _, answer := range tc.answers {
				node.Send(answer)
			}
			assertOutcome(t, answered(t, done), tc.want, tc.reason)
		})
	}
}

func TestHostSettlesACommandItsSessionLeftOpen(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		answers []map[string]any
		want    func(error) bool
		reason  string
	}{
		{"queued when the node closes", []map[string]any{receipt("c-1", "queued", "")}, is(app.ErrCommandRefused), "session_ending"},
		{"accepted when the node closes", []map[string]any{receipt("c-1", "accepted", "")}, actionport.IsUnknownOutcome, "the session ended"},
		{"sent with no answer when the node closes", nil, actionport.IsUnknownOutcome, "the session ended"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			host, node := commandingHost(t)
			done := dispatch(host, pumpCommand("c-1"))
			node.Expect("DCMD")
			for _, answer := range tc.answers {
				node.Send(answer)
			}
			node.Send(map[string]any{"type": "NSYNC", "lane": "control", "through_seq": 0, "world_time_ns": birthNS})
			node.Expect("HSYNC")
			node.Close()
			assertOutcome(t, answered(t, done), tc.want, tc.reason)
		})
	}
}

func TestHostCallsACommandUnknownWhenNoAnswerComesBeforeItsDeadline(t *testing.T) {
	t.Parallel()
	host, node := commandingHost(t)
	done := dispatch(host, pumpCommand("c-1"))
	node.Expect("DCMD")
	host.clock.Advance(20*time.Second + 2*domain.HeartbeatInterval)
	assertOutcome(t, answered(t, done), actionport.IsUnknownOutcome, "deadline")
}

func TestHostSendsNothingItCannotSendSafely(t *testing.T) {
	t.Parallel()
	host, _ := commandingHost(t)
	cases := []struct {
		name    string
		command actionport.Command
		want    func(error) bool
	}{
		{"a route the catalog does not name", withRoute(pumpCommand("c-1"), "open_valve"), contains("not in the device capability catalog")},
		{"a target the route does not bind", withTarget(pumpCommand("c-2"), "station-9"), contains("is not bound")},
		{"a preset the route does not offer", withPayload(pumpCommand("c-3"), "full"), contains("is not an allowed preset")},
	}
	for _, tc := range cases {
		if got := answered(t, dispatch(host, tc.command)); !tc.want(got.err) {
			t.Errorf("%s: DispatchAuthorized = %v", tc.name, got.err)
		}
	}
	if _, err := host.host.Dispatch(t.Context(), pumpCommand("c-4")); !errors.Is(err, app.ErrAuthorizationRequired) {
		t.Errorf("Dispatch without authorization = %v, want ErrAuthorizationRequired", err)
	}
	refuse := actionport.Authorization{Check: func(context.Context) error { return errors.New("interlock tripped") }}
	if _, err := host.host.DispatchAuthorized(t.Context(), pumpCommand("c-5"), refuse); err == nil || !strings.Contains(err.Error(), "interlock tripped") {
		t.Errorf("DispatchAuthorized under a failing check = %v, want the check's refusal", err)
	}
}

func TestHostRefusesACommandWithoutALiveSessionOrABornDevice(t *testing.T) {
	t.Parallel()
	host := pumpHost(t, commandingConfig(t, newDeviceAuthority(t)), nil)
	if got := answered(t, dispatch(host, pumpCommand("c-1"))); !errors.Is(got.err, domain.ErrNoSession) {
		t.Fatalf("a command with no session = %v, want ErrNoSession", got.err)
	}
	bornNode(t, host, "s-1")
	if got := answered(t, dispatch(host, pumpCommand("c-2"))); !errors.Is(got.err, domain.ErrDeviceNotBorn) {
		t.Fatalf("a command before the device's birth = %v, want ErrDeviceNotBorn", got.err)
	}
	if !host.host.OwnsRoute("set_pump_speed") || host.host.OwnsRoute(actionport.WatchInstallRoute) || host.host.OwnsRoute("open_valve") {
		t.Fatal("the host must own exactly the catalog's routes")
	}
}

func assertOutcome(t *testing.T, got dispatched, want func(error) bool, reason string) {
	t.Helper()
	if !want(got.err) || !strings.Contains(got.err.Error(), reason) {
		t.Fatalf("outcome = %v, want an error naming %q of the expected kind", got.err, reason)
	}
	if outcome := got.effect.ProviderResult["plant_wire"].(map[string]any); outcome["command_id"] != "c-1" {
		t.Fatalf("provider result = %v, want the command's record", outcome)
	}
}

func receiptOnBoot(id, boot string) map[string]any {
	record := receipt(id, "accepted", "")
	record["boot_id"] = boot
	return record
}

func contains(text string) func(error) bool {
	return func(err error) bool { return err != nil && strings.Contains(err.Error(), text) }
}

func withRoute(command actionport.Command, route string) actionport.Command {
	command.EffectorRoute = route
	return command
}

func withTarget(command actionport.Command, target string) actionport.Command {
	command.NormalizedTarget = target
	return command
}

func withPayload(command actionport.Command, speed string) actionport.Command {
	command.Payload = map[string]any{"speed": speed}
	return command
}
