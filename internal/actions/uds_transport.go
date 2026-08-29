package actions

import (
	"bufio"
	"context"
	"fmt"
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
	writeMu sync.Mutex
	conn    net.Conn
	reader  *bufio.Reader
}

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
	return &UDSTransport{conn: conn, reader: bufio.NewReaderSize(conn, maxDeviceFrameBytes)}, nil
}

// NewUDSTransport wraps an already-connected gateway link. It is used by tests
// and by callers that own connection setup.
func NewUDSTransport(conn net.Conn) *UDSTransport {
	return &UDSTransport{conn: conn, reader: bufio.NewReaderSize(conn, maxDeviceFrameBytes)}
}

// Send writes one already-encoded device frame (it includes its trailing
// newline). Callers serialize send/receive; the write mutex only guards against
// an interleaved QueryState control write.
func (t *UDSTransport) Send(ctx context.Context, frame []byte) error {
	t.writeMu.Lock()
	defer t.writeMu.Unlock()
	t.applyWriteDeadline(ctx)
	if _, err := t.conn.Write(frame); err != nil {
		return fmt.Errorf("send device frame: %w", err)
	}
	return nil
}

// Receive reads one newline-delimited device frame.
func (t *UDSTransport) Receive(ctx context.Context) ([]byte, error) {
	return t.readFrame(ctx)
}

// QueryState requests a fresh device.state and reads the reply.
func (t *UDSTransport) QueryState(ctx context.Context) ([]byte, error) {
	t.writeMu.Lock()
	t.applyWriteDeadline(ctx)
	_, err := t.conn.Write([]byte(queryStateControl))
	t.writeMu.Unlock()
	if err != nil {
		return nil, fmt.Errorf("request device state: %w", err)
	}
	return t.readFrame(ctx)
}

// Close closes the gateway link.
func (t *UDSTransport) Close() error {
	return t.conn.Close()
}

func (t *UDSTransport) readFrame(ctx context.Context) ([]byte, error) {
	if deadline, ok := ctx.Deadline(); ok {
		_ = t.conn.SetReadDeadline(deadline)
	} else {
		_ = t.conn.SetReadDeadline(time.Time{})
	}
	line, err := t.reader.ReadBytes('\n')
	if err != nil {
		return nil, fmt.Errorf("receive device frame: %w", err)
	}
	return line, nil
}

func (t *UDSTransport) applyWriteDeadline(ctx context.Context) {
	if deadline, ok := ctx.Deadline(); ok {
		_ = t.conn.SetWriteDeadline(deadline)
	} else {
		_ = t.conn.SetWriteDeadline(time.Time{})
	}
}
