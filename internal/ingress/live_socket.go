package ingress

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"sync"
	"sync/atomic"

	"github.com/ghassan-ai-projects/agentic-stream/internal/clock"
	"github.com/ghassan-ai-projects/agentic-stream/internal/contractsv1"
	"github.com/ghassan-ai-projects/agentic-stream/internal/eventlog"
	"github.com/ghassan-ai-projects/agentic-stream/internal/ids"
	"github.com/ghassan-ai-projects/agentic-stream/internal/telemetry"
)

const (
	defaultLiveSocketQueueSize = 128
	maxLiveSocketClients       = 16
	maxLiveSocketLineBytes     = 64 * 1024
	liveSocketSourceTag        = "live-uds"
)

var errLiveSocketLineTooLarge = errors.New("live ingress line exceeds maximum size")

// EnvelopeSink receives one validated normalized envelope from a live source.
// The sink owns appending the envelope and advancing the runtime pipeline.
type EnvelopeSink func(context.Context, contractsv1.Envelope) error

// LiveUDSSource reads normalized JSONL envelopes from a Unix-domain socket.
// It accepts reconnecting clients, applies bounded backpressure, quarantines
// malformed input, and never treats a client disconnect as a runtime failure.
type LiveUDSSource struct {
	log           *eventlog.EventLog
	tenantID      string
	path          string
	instanceID    string
	clk           clock.Clock
	logger        *slog.Logger
	telemetry     *telemetry.Runtime
	queueSize     int
	clientCount   atomic.Int64
	connection    atomic.Uint64
	connectionsMu sync.Mutex
	connections   map[net.Conn]struct{}
}

// NewLiveUDSSource creates a live normalized-event source. The source does not
// start listening until Run is called.
func NewLiveUDSSource(log *eventlog.EventLog, tenantID, path string) *LiveUDSSource {
	if tenantID == "" {
		tenantID = "default"
	}
	return &LiveUDSSource{
		log:         log,
		tenantID:    tenantID,
		path:        path,
		instanceID:  ids.Random().New("live_uds_"),
		clk:         clock.Physical(),
		logger:      slog.Default(),
		queueSize:   defaultLiveSocketQueueSize,
		connections: make(map[net.Conn]struct{}),
	}
}

// WithLogger uses logger for structured rejection and connection diagnostics.
func (s *LiveUDSSource) WithLogger(logger *slog.Logger) *LiveUDSSource {
	if logger != nil {
		s.logger = logger
	}
	return s
}

// WithTelemetry records accepted and rejected live-source lines in the
// process-local low-cardinality runtime counters.
func (s *LiveUDSSource) WithTelemetry(runtimeTelemetry *telemetry.Runtime) *LiveUDSSource {
	s.telemetry = runtimeTelemetry
	return s
}

// Run listens for live normalized JSONL until ctx is canceled or the sink
// returns an error. A canceled context is a normal shutdown and returns nil.
func (s *LiveUDSSource) Run(ctx context.Context, sink EnvelopeSink) error {
	if err := s.prepare(sink); err != nil {
		return err
	}
	listener, err := listenLiveSocket(s.path)
	if err != nil {
		return fmt.Errorf("listen live ingress socket: %w", err)
	}
	defer func() { _ = listener.Close() }()
	return s.serve(ctx, listener, sink)
}

// prepare validates the source and sink and fills the runtime defaults.
func (s *LiveUDSSource) prepare(sink EnvelopeSink) error {
	if err := s.checkConfigured(sink); err != nil {
		return err
	}
	if s.clk == nil {
		s.clk = clock.Physical()
	}
	if s.logger == nil {
		s.logger = slog.Default()
	}
	s.ensureConnections()
	s.queueSize = cmp.Or(max(s.queueSize, 0), defaultLiveSocketQueueSize)
	return nil
}

func (s *LiveUDSSource) ensureConnections() {
	s.connectionsMu.Lock()
	defer s.connectionsMu.Unlock()
	if s.connections == nil {
		s.connections = make(map[net.Conn]struct{})
	}
}

// checkConfigured requires the source, its event log, a sink and a safe
// socket path.
func (s *LiveUDSSource) checkConfigured(sink EnvelopeSink) error {
	if s == nil {
		return fmt.Errorf("live UDS source is nil")
	}
	if s.log == nil {
		return fmt.Errorf("live UDS source event log is required")
	}
	if sink == nil {
		return fmt.Errorf("live UDS source sink is required")
	}
	return validateLiveSocketPath(s.path)
}

// serve accepts clients and processes their lines in arrival order until ctx
// ends, the sink fails, or accepting fails. Shutdown closes the listener and
// every client before returning.
func (s *LiveUDSSource) serve(ctx context.Context, listener net.Listener, sink EnvelopeSink) error {
	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	lines := make(chan liveLine, s.queueSize)
	acceptDone := make(chan struct{})
	acceptErr := make(chan error, 1)
	var clients sync.WaitGroup
	go s.acceptClients(runCtx, listener, lines, acceptDone, acceptErr, &clients)
	defer func() {
		cancel()
		_ = listener.Close()
		s.closeClients()
		clients.Wait()
		<-acceptDone
	}()
	return s.processLines(ctx, runCtx, lines, acceptDone, acceptErr, sink)
}

// processLines handles queued lines in arrival order until the accept loop
// ends or the caller's context is done; normal shutdown is not an error.
func (s *LiveUDSSource) processLines(ctx, runCtx context.Context, lines <-chan liveLine, acceptDone <-chan struct{}, acceptErr <-chan error, sink EnvelopeSink) error {
	for {
		select {
		case item := <-lines:
			if err := s.processLine(runCtx, item, sink); err != nil {
				return liveLineFailure(ctx, err)
			}
		case <-acceptDone:
			return firstError(acceptErr)
		case <-ctx.Done():
			return nil
		}
	}
}

func liveLineFailure(ctx context.Context, err error) error {
	if normalLiveSocketShutdown(ctx, err) {
		return nil
	}
	return fmt.Errorf("process live ingress line: %w", err)
}

// firstError returns a pending error, or nil when none was sent.
func firstError(errs <-chan error) error {
	select {
	case err := <-errs:
		return err
	default:
		return nil
	}
}

func normalLiveSocketShutdown(ctx context.Context, err error) bool {
	return ctx.Err() != nil && (errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded))
}

type liveLine struct {
	connectionID uint64
	lineNumber   uint64
	data         []byte
	readErr      error
}
