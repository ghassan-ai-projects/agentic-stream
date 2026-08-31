package ingress

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

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

// WithClock uses clk for quarantine timestamps.
func (s *LiveUDSSource) WithClock(clk clock.Clock) *LiveUDSSource {
	if clk != nil {
		s.clk = clk
	}
	return s
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

// WithQueueSize changes the bounded line buffer. Non-positive values retain
// the safe default.
func (s *LiveUDSSource) WithQueueSize(size int) *LiveUDSSource {
	if size > 0 {
		s.queueSize = size
	}
	return s
}

// Run listens for live normalized JSONL until ctx is canceled or the sink
// returns an error. A canceled context is a normal shutdown and returns nil.
func (s *LiveUDSSource) Run(ctx context.Context, sink EnvelopeSink) error {
	if s == nil {
		return fmt.Errorf("live UDS source is nil")
	}
	if s.log == nil {
		return fmt.Errorf("live UDS source event log is required")
	}
	if sink == nil {
		return fmt.Errorf("live UDS source sink is required")
	}
	if err := validateLiveSocketPath(s.path); err != nil {
		return err
	}
	if s.clk == nil {
		s.clk = clock.Physical()
	}
	if s.logger == nil {
		s.logger = slog.Default()
	}
	s.connectionsMu.Lock()
	if s.connections == nil {
		s.connections = make(map[net.Conn]struct{})
	}
	s.connectionsMu.Unlock()
	queueSize := s.queueSize
	if queueSize <= 0 {
		queueSize = defaultLiveSocketQueueSize
	}

	listener, err := listenLiveSocket(s.path)
	if err != nil {
		return fmt.Errorf("listen live ingress socket: %w", err)
	}
	defer func() { _ = listener.Close() }()

	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	lines := make(chan liveLine, queueSize)
	acceptDone := make(chan struct{})
	acceptErr := make(chan error, 1)
	var clients sync.WaitGroup

	go s.acceptClients(runCtx, listener, lines, acceptDone, acceptErr, &clients)
	cleanup := func() {
		cancel()
		_ = listener.Close()
		s.closeClients()
		clients.Wait()
		<-acceptDone
	}
	defer cleanup()

	for {
		select {
		case item := <-lines:
			if err := s.processLine(runCtx, item, sink); err != nil {
				if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
					return nil
				}
				return fmt.Errorf("process live ingress line: %w", err)
			}
		case <-acceptDone:
			select {
			case acceptErr := <-acceptErr:
				return acceptErr
			default:
				return nil
			}
		case <-ctx.Done():
			return nil
		}
	}
}

type liveLine struct {
	connectionID uint64
	lineNumber   uint64
	data         []byte
	readErr      error
}

func (s *LiveUDSSource) acceptClients(ctx context.Context, listener net.Listener, lines chan<- liveLine, done chan<- struct{}, acceptErr chan<- error, clients *sync.WaitGroup) {
	defer close(done)
	for {
		conn, err := listener.Accept()
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			var networkErr net.Error
			if errors.As(err, &networkErr) && networkErr.Timeout() {
				timer := time.NewTimer(5 * time.Millisecond)
				select {
				case <-ctx.Done():
					if !timer.Stop() {
						<-timer.C
					}
					return
				case <-timer.C:
				}
				continue
			}
			acceptErr <- fmt.Errorf("accept live ingress client: %w", err)
			return
		}
		if s.clientCount.Load() >= maxLiveSocketClients {
			s.logger.WarnContext(ctx, "live ingress client rejected", "source", liveSocketSourceTag, "reason_code", "client_limit")
			_ = conn.Close()
			continue
		}
		connectionID := s.connection.Add(1)
		s.clientCount.Add(1)
		s.addClient(conn)
		clients.Add(1)
		go func() {
			defer clients.Done()
			defer s.clientCount.Add(-1)
			defer s.removeClient(conn)
			s.readClient(ctx, conn, connectionID, lines)
		}()
	}
}

func (s *LiveUDSSource) readClient(ctx context.Context, conn net.Conn, connectionID uint64, lines chan<- liveLine) {
	reader := bufio.NewReaderSize(conn, maxLiveSocketLineBytes)
	var lineNumber uint64
	for {
		line, err := readLiveLine(reader)
		if err != nil {
			if errors.Is(err, io.EOF) {
				return
			}
			lineNumber++
			select {
			case lines <- liveLine{connectionID: connectionID, lineNumber: lineNumber, data: line, readErr: err}:
			case <-ctx.Done():
			}
			return
		}
		lineNumber++
		if len(strings.TrimSpace(string(line))) == 0 {
			continue
		}
		select {
		case lines <- liveLine{connectionID: connectionID, lineNumber: lineNumber, data: line}:
		case <-ctx.Done():
			return
		}
	}
}

func readLiveLine(reader *bufio.Reader) ([]byte, error) {
	line := make([]byte, 0, min(maxLiveSocketLineBytes, reader.Size()))
	for {
		part, err := reader.ReadSlice('\n')
		if len(line)+len(part) > maxLiveSocketLineBytes {
			// Preserve the bounded prefix for quarantine. The complete malformed
			// frame is intentionally not retained or allowed to grow memory.
			return line, errLiveSocketLineTooLarge
		}
		line = append(line, part...)
		if err == nil {
			return line, nil
		}
		if errors.Is(err, bufio.ErrBufferFull) {
			continue
		}
		if errors.Is(err, io.EOF) && len(line) > 0 {
			return line, nil
		}
		return nil, fmt.Errorf("read live ingress line: %w", err)
	}
}

func (s *LiveUDSSource) processLine(ctx context.Context, item liveLine, sink EnvelopeSink) error {
	if item.readErr != nil {
		return s.rejectRaw(ctx, item, "line_too_large", item.readErr)
	}
	var env contractsv1.Envelope
	if err := json.Unmarshal(item.data, &env); err != nil {
		return s.rejectRaw(ctx, item, "malformed_json", err)
	}
	if env.TenantID == "" {
		env.TenantID = s.tenantID
	}
	if err := contractsv1.ValidateEnvelope(env, s.tenantID); err != nil {
		return s.rejectEnvelope(ctx, item, env, "envelope_invalid", err)
	}
	if err := s.log.ValidateEnvelope(ctx, env); err != nil {
		return s.rejectEnvelope(ctx, item, env, "schema_invalid", err)
	}
	if s.telemetry != nil {
		s.telemetry.ObserveLiveLineIngested()
	}
	return sink(ctx, env)
}

func (s *LiveUDSSource) rejectRaw(ctx context.Context, item liveLine, reason string, cause error) error {
	if s.telemetry != nil {
		s.telemetry.ObserveLiveLineRejected()
	}
	s.logger.WarnContext(ctx, "live ingress line rejected", "source", liveSocketSourceTag, "connection_id", item.connectionID, "line_number", item.lineNumber, "reason_code", reason, "error", cause)
	eventID := fmt.Sprintf("live-uds:%s:%d:%d", s.instanceID, item.connectionID, item.lineNumber)
	if err := s.log.QuarantineRaw(ctx, s.tenantID, eventID, item.data, reason, s.clk.Now().UTC().Format(time.RFC3339Nano)); err != nil {
		return fmt.Errorf("quarantine live line: %w", err)
	}
	return nil
}

func (s *LiveUDSSource) rejectEnvelope(ctx context.Context, item liveLine, env contractsv1.Envelope, reason string, cause error) error {
	if s.telemetry != nil {
		s.telemetry.ObserveLiveLineRejected()
	}
	s.logger.WarnContext(ctx, "live ingress line rejected", "source", liveSocketSourceTag, "connection_id", item.connectionID, "line_number", item.lineNumber, "reason_code", reason, "error", cause)
	if err := s.log.QuarantineEnvelope(ctx, s.tenantID, env, reason, s.clk.Now().UTC().Format(time.RFC3339Nano)); err != nil {
		return fmt.Errorf("quarantine live envelope: %w", err)
	}
	return nil
}

func (s *LiveUDSSource) addClient(conn net.Conn) {
	s.connectionsMu.Lock()
	defer s.connectionsMu.Unlock()
	s.connections[conn] = struct{}{}
}

func (s *LiveUDSSource) removeClient(conn net.Conn) {
	s.connectionsMu.Lock()
	delete(s.connections, conn)
	s.connectionsMu.Unlock()
	_ = conn.Close()
}

func (s *LiveUDSSource) closeClients() {
	s.connectionsMu.Lock()
	clients := make([]net.Conn, 0, len(s.connections))
	for conn := range s.connections {
		clients = append(clients, conn)
	}
	s.connectionsMu.Unlock()
	for _, conn := range clients {
		_ = conn.Close()
	}
}

func validateLiveSocketPath(path string) error {
	if path == "" || !filepath.IsAbs(path) || strings.Contains(path, "\x00") || strings.Contains(path, "://") || filepath.Clean(path) != path {
		return fmt.Errorf("live socket must be a clean absolute Unix path")
	}
	return nil
}

func listenLiveSocket(path string) (net.Listener, error) {
	if info, err := os.Lstat(path); err == nil {
		if info.Mode()&os.ModeSymlink != 0 || info.Mode()&os.ModeSocket == 0 {
			return nil, fmt.Errorf("refusing unsafe existing live socket path")
		}
		probeCtx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		probe, dialErr := (&net.Dialer{}).DialContext(probeCtx, "unix", path)
		cancel()
		if dialErr == nil {
			_ = probe.Close()
			return nil, fmt.Errorf("live socket is already active")
		}
		return nil, fmt.Errorf("live socket path is occupied or stale: %w", dialErr)
	} else if !os.IsNotExist(err) {
		return nil, fmt.Errorf("inspect live socket path: %w", err)
	}
	listener, err := (&net.ListenConfig{}).Listen(context.Background(), "unix", path)
	if err != nil {
		return nil, fmt.Errorf("listen Unix socket: %w", err)
	}
	if err := os.Chmod(path, 0o600); err != nil { //nolint:gosec // The live socket is intentionally owner-only.
		_ = listener.Close()
		_ = os.Remove(path)
		return nil, fmt.Errorf("secure live socket: %w", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		_ = listener.Close()
		return nil, fmt.Errorf("stat live socket: %w", err)
	}
	return &cleanLiveListener{Listener: listener, path: path, info: info}, nil
}

type cleanLiveListener struct {
	net.Listener
	path string
	info os.FileInfo
	once sync.Once
	err  error
}

func (l *cleanLiveListener) Close() error {
	l.once.Do(func() {
		l.err = l.Listener.Close()
		current, statErr := os.Stat(l.path)
		if statErr == nil && os.SameFile(l.info, current) {
			if removeErr := os.Remove(l.path); removeErr != nil && !os.IsNotExist(removeErr) {
				if l.err != nil {
					l.err = errors.Join(l.err, fmt.Errorf("remove live socket: %w", removeErr))
				} else {
					l.err = fmt.Errorf("remove live socket: %w", removeErr)
				}
			}
		} else if statErr != nil && !os.IsNotExist(statErr) && l.err == nil {
			l.err = fmt.Errorf("inspect live socket during cleanup: %w", statErr)
		}
	})
	return l.err
}
