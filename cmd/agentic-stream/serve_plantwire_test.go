package main

import (
	"strings"
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/plantwire"
	"github.com/ghassan-ai-projects/agentic-stream/internal/sources"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage/storagetest"
)

func TestServeValidatesThePlantWireFlags(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name   string
		mutate func(*serveFlags)
		want   string
	}{
		{"an emulator host of simulated nodes", func(*serveFlags) {}, ""},
		{"a physical mixed host with consent and a board", func(f *serveFlags) {
			f.effectProfile, f.effect.LiveActuation, f.effect.OwnerAuthorized = "physical", true, true
			f.plantWire.environment, f.plantWire.hardwareBoards = plantwire.EnvironmentMixed, []string{"mega-01"}
		}, ""},
		{"plant wire flags without the socket", func(f *serveFlags) {
			f.effect.PlantWireSocket, f.effect.DeviceCatalog, f.liveSocket, f.effectProfile = "", "", "/tmp/live.sock", "simulated"
		}, "require --plant-wire-socket"},
		{"a socket without a device catalog", func(f *serveFlags) { f.effect.DeviceCatalog = "" }, "requires --device-catalog"},
		{"a socket without kind files", func(f *serveFlags) { f.plantWire.kindsDir = "" }, "--plant-wire-socket requires --plant-wire-kinds"},
		{"a socket without a spec", func(f *serveFlags) { f.specPath = "" }, "--spec and one of --trace, --live-socket or --plant-wire-socket"},
		{"a socket with a trace", func(f *serveFlags) { f.tracePath = "trace.jsonl" }, "--trace is mutually exclusive"},
		{"a socket in the simulated profile", func(f *serveFlags) { f.effectProfile = "simulated" }, "simulated effect profile cannot configure"},
		{"both effect links", func(f *serveFlags) { f.effect.DeviceSocket = "/tmp/device.sock" }, "one effect link per process"},
		{"a firmware digest for plant wire", func(f *serveFlags) { f.effect.AllowedFirmwareDigests = []string{"sha256:00"} }, "--device-firmware-digest applies to --device-socket only"},
		{"a mixed host in the emulator profile", func(f *serveFlags) { f.plantWire.environment = plantwire.EnvironmentMixed }, "physical effect profile with live actuation"},
		{"hardware boards on a simulated host", func(f *serveFlags) { f.plantWire.hardwareBoards = []string{"mega-01"} }, "hardware boards need a hardware or mixed host"},
		{"a board name with prose", func(f *serveFlags) {
			f.effectProfile, f.effect.LiveActuation, f.effect.OwnerAuthorized = "physical", true, true
			f.plantWire.environment, f.plantWire.hardwareBoards = plantwire.EnvironmentHardware, []string{"the mega board"}
		}, "not a plant wire id"},
		{"a negative epoch", func(f *serveFlags) { f.plantWire.epoch = -1 }, "authority epoch is negative"},
		{"a negative dispatch lease", func(f *serveFlags) { f.effect.DispatchLease = -time.Second }, "--dispatch-lease must not be negative"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			flags := plantWireServeFlags()
			tc.mutate(&flags)
			_, err := flags.validate()
			if tc.want == "" && err != nil || tc.want != "" && (err == nil || !strings.Contains(err.Error(), tc.want)) {
				t.Fatalf("validate = %v, want %q", err, tc.want)
			}
		})
	}
}

func plantWireServeFlags() serveFlags {
	flags := serveFlags{
		liveFlags:     liveFlags{dbPath: "runtime.db", specPath: "spec.yaml", traceFormat: "normalized", effectProfile: "emulator"},
		plantWire:     plantWireFlags{environment: plantwire.EnvironmentSimulated, kindsDir: "kinds"},
		listenAddress: "127.0.0.1:8080", pollInterval: time.Second,
	}
	flags.effect.PlantWireSocket, flags.effect.DeviceCatalog = "/tmp/plantwire/host.sock", "catalog.json"
	return flags
}

func TestThePlantWireHostReadsReasoningInFlightFromTheRuntimeDatabase(t *testing.T) {
	t.Parallel()
	core := &runtimeCore{db: storagetest.OpenTempWithoutForeignKeys(t)}
	reasoningInFlight := core.reasoningInFlight(sources.NewVirtual(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)))
	if inFlight, err := reasoningInFlight(t.Context()); err != nil || inFlight {
		t.Fatalf("empty ledger: in flight = %v err=%v", inFlight, err)
	}
	digest := make([]byte, 32)
	if _, err := core.db.ExecContext(t.Context(), `INSERT INTO scheduler_items (
		scheduler_item_id, trigger_id, tenant_id, situation_id, situation_version, lane, priority, status, dedupe_key, expires_at, created_at, updated_at
		) VALUES ('i1', 't1', 'tenant', 's', 1, 'fast', 1, 'pending', ?, 'later', 'now', 'now')`, digest); err != nil {
		t.Fatal(err)
	}
	if inFlight, err := reasoningInFlight(t.Context()); err != nil || !inFlight {
		t.Fatalf("a pending item: in flight = %v err=%v", inFlight, err)
	}
}

func TestServeTimesSchedulingOnTheWallUnlessAPlantWireWorldTakesTheDecisionClock(t *testing.T) {
	t.Parallel()
	withoutSocket := plantWireServeFlags()
	withoutSocket.effect.PlantWireSocket = ""
	if decisionClockOf(withoutSocket) != nil || (effects{}).decisionClock() != nil {
		t.Fatal("a serve without a plant wire socket got a decision clock of its own")
	}
	flags := plantWireServeFlags()
	decision := decisionClockOf(flags)
	if decision == nil || decision.FollowsWorld() || (effects{decision: decision}).decisionClock() == nil {
		t.Fatalf("a plant wire serve decision clock = %v, want one that still reads the wall", decision)
	}
	if worldOf(flags, decision) != decision {
		t.Fatal("the plant wire host was not given the decision clock to bind to a world")
	}
	flags.liveSocket = "/tmp/live.sock"
	if worldOf(flags, decision) != nil {
		t.Fatal("a live socket beside the plant wire host still let a world take the decision clock")
	}
}
