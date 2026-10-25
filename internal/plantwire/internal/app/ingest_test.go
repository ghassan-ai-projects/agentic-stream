package app_test

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/contractsv1"
	"github.com/ghassan-ai-projects/agentic-stream/internal/plantwire/internal/app"
	"github.com/ghassan-ai-projects/agentic-stream/internal/plantwire/internal/transport"
	"github.com/ghassan-ai-projects/agentic-stream/internal/testsupport/plantwirefake"
)

const (
	birthNS   = int64(1767225600000000000)
	firstBoot = "board-01-boot-1"
	second    = int64(time.Second)
)

func pumpHost(t *testing.T, config app.Config, emit app.Emit) runningHost {
	t.Helper()
	config.Settings, config.KindsDir, config.SitePath = simulatedHost, plantwirefake.KindsDir(t, "."), siteFile(t, pumpSite())
	return startHostWith(t, config, emit)
}

func TestHostFeedsTheReadingsAndHeartbeatsTheSiteFileMaps(t *testing.T) {
	t.Parallel()
	host := pumpHost(t, app.Config{}, nil)
	node := bornNode(t, host, "s-1")
	node.Send(plantwirefake.Fixture(t, ".", "DBIRTH.json"))
	assertEvent(t, host.event(t), "pump.heartbeat.observed", birthNS, map[string]any{"quality": "valid", "boot_id": firstBoot, "seq": uint64(1)})
	node.Send(pumpData(firstBoot, birthNS+60*second, "good", false))
	reading := host.event(t)
	assertEvent(t, reading, "pump.flow.observed", birthNS+60*second, map[string]any{"m3_per_hour": 42.5, "quality": "valid", "seq": uint64(2), "schema_version": "1.0"})
	if !strings.HasPrefix(reading.ID, "pw-") || reading.Source != "plantwire-test" || len(reading.Quality) != 0 {
		t.Errorf("reading = %+v, want a pw- id from source plantwire-test without quality flags", reading)
	}
	assertEvent(t, host.event(t), "pump.heartbeat.observed", birthNS+60*second, map[string]any{"seq": uint64(2)})
	node.Send(pumpData(firstBoot, birthNS+120*second, "good", true))
	node.Send(pumpData(firstBoot, birthNS+65*second, "out_of_range", false))
	broken := host.event(t)
	assertEvent(t, broken, "pump.flow.observed", birthNS+65*second, map[string]any{"quality": "invalid", "seq": uint64(4)})
	if _, carried := broken.Data["m3_per_hour"]; carried || len(broken.Quality) != 1 || broken.Quality[0].Code != "invalid" {
		t.Errorf("an out_of_range sample gave %+v, want no value and one invalid quality flag", broken)
	}
}

func TestHostFollowsADeviceThroughItsRebootOnly(t *testing.T) {
	t.Parallel()
	host := pumpHost(t, app.Config{}, nil)
	node := bornNode(t, host, "s-1")
	node.Send(plantwirefake.Fixture(t, ".", "DBIRTH.json"))
	host.event(t)
	node.Send(plantwirefake.Fixture(t, ".", "DDEATH.json"))
	node.Send(pumpData(firstBoot, birthNS+second, "good", false))
	reborn := plantwirefake.Fixture(t, ".", "DBIRTH.json")
	reborn["boot_id"] = "board-01-boot-2"
	node.Send(reborn)
	node.Send(pumpData("board-01-boot-2", birthNS+2*second, "good", false))
	assertEvent(t, host.event(t), "pump.heartbeat.observed", birthNS, map[string]any{"boot_id": "board-01-boot-2"})
	assertEvent(t, host.event(t), "pump.flow.observed", birthNS+2*second, map[string]any{"boot_id": "board-01-boot-2"})
}

func TestHostEndsTheSessionWhenThePipelineFallsBehind(t *testing.T) {
	t.Parallel()
	stalled := func(ctx context.Context, _ contractsv1.Envelope) error {
		<-ctx.Done()
		return nil
	}
	host := pumpHost(t, app.Config{QueueSize: 1}, stalled)
	node := bornNode(t, host, "s-1")
	node.Send(plantwirefake.Fixture(t, ".", "DBIRTH.json"))
	node.Send(pumpData(firstBoot, birthNS+60*second, "good", false))
	if !node.Closed() {
		t.Fatal("the host kept the session while its ingest queue was full")
	}
}

func TestRunStopsWithTheErrorOfAFailingPipeline(t *testing.T) {
	t.Parallel()
	failure := errors.New("pipeline down")
	socket, err := (&net.ListenConfig{}).Listen(t.Context(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	host, err := app.New(app.Config{Listener: transport.NewListener(socket), Settings: simulatedHost, KindsDir: plantwirefake.KindsDir(t, "."), SitePath: siteFile(t, pumpSite())})
	if err != nil {
		t.Fatalf("New = %v", err)
	}
	done := make(chan error, 1)
	go func() {
		done <- host.Run(t.Context(), func(context.Context, contractsv1.Envelope) error { return failure })
	}()
	node := plantwirefake.Dial(t, "tcp", socket.Addr().String(), "s-1")
	node.Send(plantwirefake.Fixture(t, ".", "NBIRTH.json"))
	node.Send(plantwirefake.Fixture(t, ".", "DBIRTH.json"))
	select {
	case err := <-done:
		if !errors.Is(err, failure) {
			t.Fatalf("Run = %v, want the pipeline's error", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Run kept serving after the pipeline failed")
	}
}

func pumpData(boot string, atNS int64, quality string, historical bool) map[string]any {
	return map[string]any{"type": "DDATA", "lane": "data", "device": "P1", "boot_id": boot, "historical": historical,
		"metrics": []any{map[string]any{"alias": 1, "value": 42.5, "quality": quality, "event_time_ns": atNS, "time_source": "device"},
			map[string]any{"alias": 99, "value": 1, "quality": "good", "event_time_ns": atNS, "time_source": "device"}}}
}

func assertEvent(t *testing.T, event contractsv1.Envelope, eventType string, atNS int64, data map[string]any) {
	t.Helper()
	if event.Type != eventType || !event.EventTime.Equal(time.Unix(0, atNS)) || event.Entity.ID != event.PartitionKey || event.Entity.ID == "" {
		t.Fatalf("event = %+v, want a %s at %d for its entity", event, eventType, atNS)
	}
	for member, want := range data {
		if got := event.Data[member]; fmt.Sprint(got) != fmt.Sprint(want) {
			t.Errorf("%s data %s = %#v, want %#v", eventType, member, got, want)
		}
	}
}
