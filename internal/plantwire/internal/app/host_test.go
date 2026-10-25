package app_test

import (
	"context"
	"encoding/json"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/contractsv1"
	"github.com/ghassan-ai-projects/agentic-stream/internal/plantwire/internal/app"
	"github.com/ghassan-ai-projects/agentic-stream/internal/plantwire/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/plantwire/internal/transport"
	"github.com/ghassan-ai-projects/agentic-stream/internal/sources"
	"github.com/ghassan-ai-projects/agentic-stream/internal/testsupport/plantwirefake"
)

const epoch = 7

var hostStart = time.Unix(1_800_000_000, 0)

var simulatedHost = domain.Settings{Environment: domain.EnvironmentSimulated, Epoch: epoch}

type runningHost struct {
	host    *app.Host
	address string
	clock   *sources.Virtual
	cancel  context.CancelFunc
	done    chan error
	once    *sync.Once
	events  chan contractsv1.Envelope
}

func startHost(t *testing.T, settings domain.Settings) runningHost {
	t.Helper()
	return startHostWith(t, app.Config{Settings: settings, KindsDir: plantwirefake.KindsDir(t, ".")}, nil)
}

func startHostWith(t *testing.T, config app.Config, emit app.Emit) runningHost {
	t.Helper()
	listener, err := (&net.ListenConfig{}).Listen(t.Context(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	config.Listener = transport.NewListener(listener)
	if config.Clock == nil {
		config.Clock = sources.NewVirtual(hostStart)
	}
	host, err := app.New(config)
	if err != nil {
		t.Fatalf("New = %v", err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	running := runningHost{host: host, address: listener.Addr().String(), clock: config.Clock.(*sources.Virtual), cancel: cancel,
		done: make(chan error, 1), once: &sync.Once{}, events: make(chan contractsv1.Envelope, 64)}
	if emit == nil {
		emit = running.collect
	}
	go func() { running.done <- host.Run(ctx, emit) }()
	t.Cleanup(func() { running.stop(t) })
	return running
}

func (r runningHost) collect(_ context.Context, event contractsv1.Envelope) error {
	r.events <- event
	return nil
}

func (r runningHost) event(t *testing.T) contractsv1.Envelope {
	t.Helper()
	select {
	case event := <-r.events:
		return event
	case <-time.After(5 * time.Second):
		t.Fatal("the host emitted no event")
		return contractsv1.Envelope{}
	}
}

func (r runningHost) stop(t *testing.T) {
	t.Helper()
	r.once.Do(func() {
		r.cancel()
		select {
		case err := <-r.done:
			if err != nil {
				t.Errorf("Run = %v after the host stopped, want nil", err)
			}
		case <-time.After(5 * time.Second):
			t.Error("Run did not return after the host stopped")
		}
	})
}

func (r runningHost) node(t *testing.T, session string) *plantwirefake.Node {
	t.Helper()
	return plantwirefake.Dial(t, "tcp", r.address, session)
}

func bornNode(t *testing.T, r runningHost, session string) *plantwirefake.Node {
	t.Helper()
	node := r.node(t, session)
	node.Send(plantwirefake.Fixture(t, ".", "NBIRTH.json"))
	state := node.Expect("STATE")
	if state["online"] != true || state["session_id"] != session || number(t, state["authority_epoch"]) != epoch {
		t.Fatalf("handshake STATE = %v, want online in session %s at epoch %d", state, session, epoch)
	}
	return node
}

func barrier(node *plantwirefake.Node) {
	node.Send(map[string]any{"type": "NSYNC", "lane": "control", "through_seq": 0, "world_time_ns": 1767225660000000000})
}

func death(node *plantwirefake.Node, bdSeq int) {
	node.Send(map[string]any{"type": "NDEATH", "lane": "control", "bd_seq": bdSeq, "reason": "shutdown"})
}

func number(t *testing.T, value any) int64 {
	t.Helper()
	parsed, err := value.(json.Number).Int64()
	if err != nil {
		t.Fatalf("%v is not an integer: %v", value, err)
	}
	return parsed
}
