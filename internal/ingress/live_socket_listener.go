package ingress

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

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
	l.once.Do(func() { l.err = l.closeAndRemove() })
	return l.err
}

// closeAndRemove closes the listener and removes the socket file only if it
// is still the one this listener created.
func (l *cleanLiveListener) closeAndRemove() error {
	closeErr := l.Listener.Close()
	if closeErr != nil {
		closeErr = fmt.Errorf("close live socket listener: %w", closeErr)
	}
	current, statErr := os.Stat(l.path)
	switch {
	case statErr == nil && os.SameFile(l.info, current):
		return errors.Join(closeErr, removeSocketFile(l.path))
	case statErr != nil && !os.IsNotExist(statErr) && closeErr == nil:
		return fmt.Errorf("inspect live socket during cleanup: %w", statErr)
	default:
		return closeErr
	}
}

func removeSocketFile(path string) error {
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove live socket: %w", err)
	}
	return nil
}
