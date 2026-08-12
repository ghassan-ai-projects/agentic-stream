package worker

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
)

// ListenEvidenceSocket creates a private runtime-owned Unix socket. It
// refuses symlinks and active sockets rather than unlinking an unknown path.
func ListenEvidenceSocket(path string) (net.Listener, error) {
	if err := ValidateEvidenceSocketPath(path); err != nil {
		return nil, err
	}
	parent := filepath.Dir(path)
	parentInfo, err := os.Lstat(parent)
	createdParent := false
	if os.IsNotExist(err) {
		if err := os.MkdirAll(parent, 0o700); err != nil {
			return nil, fmt.Errorf("create private socket directory: %w", err)
		}
		parentInfo, err = os.Lstat(parent)
		createdParent = true
	}
	if err != nil {
		return nil, fmt.Errorf("inspect socket directory: %w", err)
	}
	if parentInfo.Mode()&os.ModeSymlink != 0 || !parentInfo.IsDir() {
		return nil, fmt.Errorf("socket parent is not a private directory")
	}
	if parentInfo.Mode().Perm()&0o077 != 0 {
		return nil, fmt.Errorf("socket parent directory is not private")
	}
	if createdParent && parentInfo.Mode().Perm() != 0o700 {
		if err := os.Chmod(parent, 0o700); err != nil { //nolint:gosec // Private directory permissions are deliberately 0700.
			return nil, fmt.Errorf("secure socket directory: %w", err)
		}
	}
	if info, err := os.Lstat(path); err == nil {
		if info.Mode()&os.ModeSymlink != 0 || info.Mode()&os.ModeSocket == 0 {
			return nil, fmt.Errorf("refusing unsafe existing socket path")
		}
		probeCtx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		probe, dialErr := (&net.Dialer{}).DialContext(probeCtx, "unix", path)
		cancel()
		if dialErr == nil {
			_ = probe.Close()
			return nil, fmt.Errorf("evidence socket is already active")
		}
		return nil, fmt.Errorf("evidence socket path is occupied or stale: %w", dialErr)
	} else if !os.IsNotExist(err) {
		return nil, fmt.Errorf("inspect evidence socket: %w", err)
	}
	listener, err := (&net.ListenConfig{}).Listen(context.Background(), "unix", path)
	if err != nil {
		return nil, fmt.Errorf("listen on evidence socket: %w", err)
	}
	if err := os.Chmod(path, 0o600); err != nil {
		_ = listener.Close()
		_ = os.Remove(path)
		return nil, fmt.Errorf("secure evidence socket: %w", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		_ = listener.Close()
		return nil, fmt.Errorf("stat evidence socket: %w", err)
	}
	return &cleanListener{Listener: listener, path: path, info: info}, nil
}

// DialEvidenceSocket dials only a Unix socket with transport credentials that
// carry no remote-network trust. Application capability validation remains
// mandatory for every call.
func DialEvidenceSocket(ctx context.Context, path string) (*grpc.ClientConn, error) {
	if err := ValidateEvidenceSocketPath(path); err != nil {
		return nil, err
	}
	conn, err := grpc.NewClient("passthrough:///evidence", grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) {
		dialCtx, cancel := context.WithTimeout(ctx, time.Second)
		defer cancel()
		return (&net.Dialer{}).DialContext(dialCtx, "unix", path)
	}), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, fmt.Errorf("dial evidence socket: %w", err)
	}
	return conn, nil
}

// DialEpisodeWorkerSocket dials a local EpisodeWorker over a private Unix
// socket. The worker protocol remains responsible for handshake and identity
// validation; this helper only constrains transport to the local socket.
func DialEpisodeWorkerSocket(ctx context.Context, path string) (*grpc.ClientConn, error) {
	return DialEpisodeWorkerSocketTLS(ctx, path, nil)
}

// DialEpisodeWorkerSocketTLS dials an EpisodeWorker over UDS using TLS when
// tlsConfig is non-nil. This supports local UDS workers and remote-style
// certificate authentication without changing the application protocol.
func DialEpisodeWorkerSocketTLS(ctx context.Context, path string, tlsConfig *tls.Config) (*grpc.ClientConn, error) {
	if err := ValidateEvidenceSocketPath(path); err != nil {
		return nil, err
	}
	transport := grpc.WithTransportCredentials(insecure.NewCredentials())
	if tlsConfig != nil {
		transport = grpc.WithTransportCredentials(credentials.NewTLS(tlsConfig))
	}
	conn, err := grpc.NewClient("passthrough:///episode-worker", grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, "unix", path)
	}), transport)
	if err != nil {
		return nil, fmt.Errorf("dial episode worker socket: %w", err)
	}
	return conn, nil
}

type cleanListener struct {
	net.Listener
	path string
	info os.FileInfo
}

func (l *cleanListener) Close() error {
	err := l.Listener.Close()
	if err != nil {
		return fmt.Errorf("close evidence listener: %w", err)
	}
	current, statErr := os.Stat(l.path)
	if statErr != nil {
		if os.IsNotExist(statErr) {
			return nil
		}
		return fmt.Errorf("inspect evidence socket during cleanup: %w", statErr)
	}
	if !os.SameFile(l.info, current) {
		return nil
	}
	if removeErr := os.Remove(l.path); removeErr != nil && !os.IsNotExist(removeErr) {
		return fmt.Errorf("remove evidence socket: %w", removeErr)
	}
	return nil
}

// ValidateEvidenceSocketPath is the v1 transport rule: only an absolute local
// filesystem path is accepted. URI schemes and remote endpoints are absent.
func ValidateEvidenceSocketPath(path string) error {
	if path == "" || !filepath.IsAbs(path) || strings.Contains(path, "\x00") || strings.Contains(path, "://") || filepath.Clean(path) != path {
		return fmt.Errorf("evidence socket must be a clean absolute Unix path")
	}
	return nil
}
