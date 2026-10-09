package app_test

import (
	"context"
	"errors"
	"sync"
)

type fakeDeviceTransport struct {
	mu           sync.Mutex
	frames       [][]byte
	sentFrames   [][]byte
	sends        int
	receiveErr   error
	sendErr      error
	closed       bool
	receiveHook  func()
	sendHook     func()
	stateQueries int
}

func (t *fakeDeviceTransport) queue(frames ...[]byte) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.frames = append(t.frames, frames...)
}

func (t *fakeDeviceTransport) Send(_ context.Context, frame []byte) error {
	t.mu.Lock()
	if t.sendErr != nil {
		t.mu.Unlock()
		return t.sendErr
	}
	t.sends++
	t.sentFrames = append(t.sentFrames, append([]byte(nil), frame...))
	hook := t.sendHook
	t.mu.Unlock()
	if hook != nil {
		hook()
	}
	return nil
}

func (t *fakeDeviceTransport) Receive(_ context.Context) ([]byte, error) {
	t.mu.Lock()
	var frame []byte
	if len(t.frames) > 0 {
		frame = t.frames[0]
		t.frames = t.frames[1:]
	}
	receiveErr := t.receiveErr
	hook := t.receiveHook
	t.mu.Unlock()
	if hook != nil {
		hook()
	}
	if frame != nil {
		return frame, nil
	}
	if receiveErr != nil {
		return nil, receiveErr
	}
	return nil, errors.New("fake transport has no queued frame")
}

func (t *fakeDeviceTransport) QueryState(ctx context.Context) ([]byte, error) {
	t.mu.Lock()
	t.stateQueries++
	t.mu.Unlock()
	return t.Receive(ctx)
}

func (t *fakeDeviceTransport) Close() error {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.closed = true
	return nil
}

func (t *fakeDeviceTransport) sendCount() int {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.sends
}

func (t *fakeDeviceTransport) isClosed() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.closed
}

func (t *fakeDeviceTransport) stateQueryCount() int {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.stateQueries
}

func (t *fakeDeviceTransport) firstSentFrame() []byte {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.sentFrames[0]
}
