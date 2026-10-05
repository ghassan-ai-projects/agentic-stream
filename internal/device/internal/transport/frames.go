package transport

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/device/internal/wire"
)

// PartialSendError reports a send that failed after some bytes may have
// reached the gateway, so the command's outcome is unknown rather than failed.
type PartialSendError struct{ Err error }

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

	return armCancellationDeadline(ctx, setDeadline, closeConnection)
}

func armCancellationDeadline(ctx context.Context, setDeadline func(time.Time) error, closeConnection func() error) (func() error, error) {
	callbackDone := make(chan struct{})
	callbackErr := make(chan error, 1)
	stop := context.AfterFunc(ctx, func() { applyCancellationDeadline(setDeadline, closeConnection, callbackDone, callbackErr) })
	return func() error { return clearOperationDeadline(stop, callbackDone, callbackErr, setDeadline) }, nil
}

func applyCancellationDeadline(setDeadline func(time.Time) error, closeConnection func() error, callbackDone chan struct{}, callbackErr chan error) {
	defer close(callbackDone)
	if err := setDeadline(time.Now()); err != nil {
		if closeErr := closeConnection(); closeErr != nil {
			err = errors.Join(err, fmt.Errorf("close canceled device transport: %w", closeErr))
		}
		callbackErr <- err
	}
}

func clearOperationDeadline(stop func() bool, callbackDone chan struct{}, callbackErr chan error, setDeadline func(time.Time) error) error {
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
}

func writeAll(conn net.Conn, frame []byte) (written int, err error) {
	for len(frame) > 0 {
		count, err := conn.Write(frame)
		if count < 0 || count > len(frame) {
			return written, fmt.Errorf("invalid write count %d", count)
		}
		written += count
		frame = frame[count:]
		if err != nil {
			return written, fmt.Errorf("write device frame: %w", err)
		}
		if count == 0 {
			return written, io.ErrShortWrite
		}
	}
	return written, nil
}

func (e *PartialSendError) Error() string {
	return "device frame may have been sent: " + e.Err.Error()
}

func (e *PartialSendError) Unwrap() error { return e.Err }

// MayHaveSent reports whether a failed send may have put bytes on the link,
// which makes the command's outcome unknown rather than failed.
func MayHaveSent(err error) bool {
	var sentErr *PartialSendError
	return errors.As(err, &sentErr)
}

func validateOutgoingFrame(frame []byte) error {
	if len(frame) == 0 {
		return fmt.Errorf("device frame is empty")
	}
	if len(frame) > wire.MaxFrameBytes {
		return fmt.Errorf("device frame exceeds %d bytes", wire.MaxFrameBytes)
	}
	if frame[len(frame)-1] != '\n' || bytes.Count(frame, []byte{'\n'}) != 1 {
		return fmt.Errorf("device frame must contain exactly one trailing newline")
	}
	return nil
}

func readBoundedFrame(reader *bufio.Reader) ([]byte, error) {
	var frame bytes.Buffer
	frame.Grow(wire.MaxFrameBytes)
	for {
		part, err := reader.ReadSlice('\n')
		if frame.Len()+len(part) > wire.MaxFrameBytes {
			return nil, fmt.Errorf("device frame exceeds %d bytes: %w", wire.MaxFrameBytes, errDeviceFrameTooLarge)
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
