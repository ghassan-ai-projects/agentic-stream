package transport

import (
	"bufio"
	"errors"
	"fmt"
	"net"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/plantwire/internal/wire"
)

const writeDeadline = 2 * time.Second

var ErrLineTooLong = errors.New("plantwire: a line is longer than the largest record")

type Conn struct {
	conn   net.Conn
	reader *bufio.Reader
}

type Listener struct{ listener net.Listener }

func NewListener(listener net.Listener) *Listener {
	if listener == nil {
		return nil
	}
	return &Listener{listener: listener}
}

func (l *Listener) Accept() (*Conn, error) {
	conn, err := l.listener.Accept()
	if err != nil {
		return nil, fmt.Errorf("plantwire: accept a node: %w", err)
	}
	return &Conn{conn: conn, reader: bufio.NewReaderSize(conn, wire.MaxBirthBytes+1)}, nil
}

func (c *Conn) ReadLine() ([]byte, error) {
	line, err := c.reader.ReadSlice('\n')
	if errors.Is(err, bufio.ErrBufferFull) {
		return nil, fmt.Errorf("%w: more than %d bytes", ErrLineTooLong, wire.MaxBirthBytes)
	}
	if err != nil {
		return nil, fmt.Errorf("plantwire: read a record: %w", err)
	}
	return append([]byte(nil), line...), nil
}

func (c *Conn) Write(line []byte) error {
	if err := c.conn.SetWriteDeadline(time.Now().Add(writeDeadline)); err != nil {
		return fmt.Errorf("plantwire: set the write deadline: %w", err)
	}
	if _, err := c.conn.Write(append(line, '\n')); err != nil {
		return fmt.Errorf("plantwire: write a record: %w", err)
	}
	return nil
}

func (l *Listener) Close() error {
	if err := l.listener.Close(); err != nil {
		return fmt.Errorf("plantwire: close the listener: %w", err)
	}
	return nil
}

func (c *Conn) Close() error {
	if err := c.conn.Close(); err != nil {
		return fmt.Errorf("plantwire: close the connection: %w", err)
	}
	return nil
}
