package actions

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"sync"
	"time"
)

// UDSTransport is a DeviceTransport over a Unix-domain-socket gateway link. The
// device end (the Streams Simulator emulator, `streamsim device serve`) speaks
// the same newline-delimited device wire records. Agentic Stream opens no serial
// port: this is the typed gateway link for the emulator effect profile, and raw
// serial framing on hardware remains the edge gateway's job behind the same
// interface.
type UDSTransport struct {
	writeGate chan struct{}
	readMu    sync.Mutex
	stateMu   sync.RWMutex
	conn      net.Conn
	reader    *bufio.Reader
	closed    bool
}

var errDeviceFrameTooLarge = errors.New("device frame exceeds maximum size")

// queryStateControl is the host→device control line that asks the device for a
// fresh state record. It mirrors the emulator's device.QueryStateControl; it is
// transport control, not one of the four device wire records.
const queryStateControl = "{\"message_type\":\"query_state\"}\n"

// DialUDSTransport connects to a device gateway listening on the Unix socket at
// path.
func DialUDSTransport(ctx context.Context, path string) (*UDSTransport, error) {
	var dialer net.Dialer
	conn, err := dialer.DialContext(ctx, "unix", path)
	if err != nil {
		return nil, fmt.Errorf("dial device gateway %s: %w", path, err)
	}
	return newUDSTransport(conn), nil
}

// NewUDSTransport wraps an already-connected gateway link. It is used by tests
// and by callers that own connection setup.
func NewUDSTransport(conn net.Conn) *UDSTransport {
	return newUDSTransport(conn)
}

func newUDSTransport(conn net.Conn) *UDSTransport {
	writeGate := make(chan struct{}, 1)
	writeGate <- struct{}{}
	return &UDSTransport{writeGate: writeGate, conn: conn, reader: bufio.NewReaderSize(conn, maxDeviceFrameBytes)}
}

// Send writes one already-encoded device frame (it includes its trailing
// newline). Callers serialize each send/receive pair; the write gate also keeps
// a QueryState control write from interleaving with another write.
func (t *UDSTransport) Send(ctx context.Context, frame []byte) (err error) {
	if err := validateOutgoingFrame(frame); err != nil {
		return fmt.Errorf("validate outgoing device frame: %w", err)
	}
	return t.writeFrame(ctx, frame)
}

func (t *UDSTransport) writeFrame(ctx context.Context, frame []byte) (err error) {
	if err := t.ensureOpen(); err != nil {
		return err
	}
	if err := t.acquireWrite(ctx); err != nil {
		return err
	}
	defer t.releaseWrite()
	return t.writeFrameWithWriteGate(ctx, frame)
}

func (t *UDSTransport) writeFrameWithWriteGate(ctx context.Context, frame []byte) (err error) {
	cleanup, err := t.prepareWrite(ctx)
	if err != nil {
		return fmt.Errorf("prepare device frame send: %w", err)
	}
	written := 0
	defer func() {
		if cleanupErr := cleanup(); cleanupErr != nil {
			resetErr := fmt.Errorf("reset device frame deadline: %w", cleanupErr)
			if written > 0 {
				resetErr = &possiblySentError{err: resetErr}
			}
			err = errors.Join(err, resetErr)
		}
	}()
	var writeErr error
	written, writeErr = writeAll(t.conn, frame)
	if writeErr != nil {
		writeErr = fmt.Errorf("send device frame: %w", contextError(ctx, writeErr))
		if written > 0 {
			writeErr = &possiblySentError{err: writeErr}
		}
		return writeErr
	}
	return nil
}

// Receive reads one newline-delimited device frame.
func (t *UDSTransport) Receive(ctx context.Context) ([]byte, error) {
	if err := t.ensureOpen(); err != nil {
		return nil, err
	}
	t.readMu.Lock()
	defer t.readMu.Unlock()
	return t.readFrame(ctx)
}

// QueryState requests a fresh device.state and reads the reply. Callers must
// serialize this request/response with any Send/Receive pair.
func (t *UDSTransport) QueryState(ctx context.Context) ([]byte, error) {
	if err := t.ensureOpen(); err != nil {
		return nil, err
	}
	// Acquire the write gate before readMu. A normal Send can hold the write
	// gate while its caller waits to Receive; taking readMu first would let a
	// concurrent QueryState block that Receive and deadlock the pair.
	if err := t.acquireWrite(ctx); err != nil {
		return nil, err
	}
	defer t.releaseWrite()
	t.readMu.Lock()
	defer t.readMu.Unlock()
	if err := t.writeFrameWithWriteGate(ctx, []byte(queryStateControl)); err != nil {
		return nil, fmt.Errorf("request device state: %w", err)
	}
	return t.readFrame(ctx)
}

// Close closes the gateway link.
func (t *UDSTransport) Close() error {
	if t == nil {
		return nil
	}
	t.stateMu.Lock()
	if t.closed {
		t.stateMu.Unlock()
		return nil
	}
	t.closed = true
	conn := t.conn
	t.stateMu.Unlock()
	if conn == nil {
		return nil
	}
	if err := conn.Close(); err != nil {
		return fmt.Errorf("close device transport: %w", err)
	}
	return nil
}

func (t *UDSTransport) readFrame(ctx context.Context) (line []byte, err error) {
	if err := t.ensureOpen(); err != nil {
		return nil, err
	}
	cleanup, err := t.prepareRead(ctx)
	if err != nil {
		return nil, fmt.Errorf("prepare device frame receive: %w", err)
	}
	defer func() { err = errors.Join(err, cleanup()) }()
	line, err = readBoundedFrame(t.reader)
	if err != nil {
		if errors.Is(err, errDeviceFrameTooLarge) {
			err = errors.Join(err, t.Close())
		}
		return nil, fmt.Errorf("receive device frame: %w", contextError(ctx, err))
	}
	return line, nil
}

func (t *UDSTransport) prepareRead(ctx context.Context) (func() error, error) {
	return prepareDeadline(ctx, t.conn.SetReadDeadline, t.Close)
}

func (t *UDSTransport) prepareWrite(ctx context.Context) (func() error, error) {
	return prepareDeadline(ctx, t.conn.SetWriteDeadline, t.Close)
}

func (t *UDSTransport) ensureOpen() error {
	if t == nil {
		return fmt.Errorf("device transport is not configured")
	}
	t.stateMu.RLock()
	defer t.stateMu.RUnlock()
	if t.closed {
		return fmt.Errorf("device transport is closed")
	}
	return nil
}

func (t *UDSTransport) acquireWrite(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return fmt.Errorf("wait for device write slot: %w", ctx.Err())
	case <-t.writeGate:
		return nil
	}
}

func (t *UDSTransport) releaseWrite() {
	t.writeGate <- struct{}{}
}

func prepareDeadline(ctx context.Context, setDeadline func(time.Time) error, closeConnection func() error) (func() error, error) {
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("prepare device operation context: %w", err)
	}
	deadline := time.Time{}
	if contextDeadline, ok := ctx.Deadline(); ok {
		deadline = contextDeadline
	}
	if err := setDeadline(deadline); err != nil {
		return nil, err
	}

	callbackDone := make(chan struct{})
	callbackErr := make(chan error, 1)
	stop := context.AfterFunc(ctx, func() {
		defer close(callbackDone)
		if err := setDeadline(time.Now()); err != nil {
			if closeErr := closeConnection(); closeErr != nil {
				err = errors.Join(err, fmt.Errorf("close canceled device transport: %w", closeErr))
			}
			callbackErr <- err
		}
	})
	return func() error {
		if !stop() {
			<-callbackDone
		}
		var cleanupErr error
		select {
		case err := <-callbackErr:
			cleanupErr = fmt.Errorf("apply cancellation deadline: %w", err)
		default:
		}
		return errors.Join(cleanupErr, setDeadline(time.Time{}))
	}, nil
}

func writeAll(conn net.Conn, frame []byte) (written int, err error) {
	for len(frame) > 0 {
		count, err := conn.Write(frame)
		if count < 0 || count > len(frame) {
			return written, fmt.Errorf("invalid write count %d", count)
		}
		written += count
		if count > 0 {
			frame = frame[count:]
		}
		if err != nil {
			return written, fmt.Errorf("write device frame: %w", err)
		}
		if count == 0 {
			return written, io.ErrShortWrite
		}
	}
	return written, nil
}

type possiblySentError struct{ err error }

func (e *possiblySentError) Error() string {
	return "device frame may have been sent: " + e.err.Error()
}

func (e *possiblySentError) Unwrap() error { return e.err }

func transportMayHaveSent(err error) bool {
	var sentErr *possiblySentError
	return errors.As(err, &sentErr)
}

func validateOutgoingFrame(frame []byte) error {
	if len(frame) == 0 {
		return fmt.Errorf("device frame is empty")
	}
	if len(frame) > maxDeviceFrameBytes {
		return fmt.Errorf("device frame exceeds %d bytes", maxDeviceFrameBytes)
	}
	if frame[len(frame)-1] != '\n' || bytes.Count(frame, []byte{'\n'}) != 1 {
		return fmt.Errorf("device frame must contain exactly one trailing newline")
	}
	return nil
}

func readBoundedFrame(reader *bufio.Reader) ([]byte, error) {
	var frame bytes.Buffer
	frame.Grow(maxDeviceFrameBytes)
	for {
		part, err := reader.ReadSlice('\n')
		if frame.Len()+len(part) > maxDeviceFrameBytes {
			return nil, fmt.Errorf("device frame exceeds %d bytes: %w", maxDeviceFrameBytes, errDeviceFrameTooLarge)
		}
		_, _ = frame.Write(part)
		if err == nil {
			return frame.Bytes(), nil
		}
		if !errors.Is(err, bufio.ErrBufferFull) {
			return nil, fmt.Errorf("read device frame: %w", err)
		}
	}
}

func contextError(ctx context.Context, err error) error {
	if contextErr := ctx.Err(); contextErr != nil {
		return fmt.Errorf("device operation context: %w", contextErr)
	}
	return err
}
