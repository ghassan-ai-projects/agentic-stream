package transport

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/ghassan-ai-projects/agentic-stream/internal/worker/internal/domain"
)

func ListenEvidenceSocket(path string) (net.Listener, error) {
	if err := prepareEvidenceSocket(path); err != nil {
		return nil, err
	}
	listener, err := (&net.ListenConfig{}).Listen(context.Background(), "unix", path)
	if err != nil {
		return nil, fmt.Errorf("listen on evidence socket: %w", err)
	}
	return secureEvidenceListener(path, listener)
}

func prepareEvidenceSocketDirectory(path string) error {
	parent := filepath.Dir(path)
	parentInfo, createdParent, err := inspectSocketParent(parent)
	if err != nil {
		return err
	}
	if parentInfo.Mode()&os.ModeSymlink != 0 || !parentInfo.IsDir() {
		return fmt.Errorf("socket parent is not a private directory")
	}
	if parentInfo.Mode().Perm()&0o077 != 0 {
		return fmt.Errorf("socket parent directory is not private")
	}
	return secureCreatedSocketDirectory(parent, parentInfo, createdParent)
}

func refuseExistingEvidenceSocket(path string) error {
	if info, err := os.Lstat(path); err == nil {
		if info.Mode()&os.ModeSymlink != 0 || info.Mode()&os.ModeSocket == 0 {
			return fmt.Errorf("refusing unsafe existing socket path")
		}
		return probeExistingEvidenceSocket(path)
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("inspect evidence socket: %w", err)
	}
	return nil
}

func DialEpisodeWorkerSocketTLS(ctx context.Context, path string, tlsConfig *tls.Config) (*grpc.ClientConn, error) {
	if err := domain.ValidateEvidenceSocketPath(path); err != nil {
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
	return l.removeOwnedSocket()
}

func prepareEvidenceSocket(path string) error {
	if err := domain.ValidateEvidenceSocketPath(path); err != nil {
		return err
	}
	if err := prepareEvidenceSocketDirectory(path); err != nil {
		return err
	}
	if err := refuseExistingEvidenceSocket(path); err != nil {
		return err
	}
	return nil
}

func secureEvidenceListener(path string, listener net.Listener) (net.Listener, error) {

	if unix, ok := listener.(*net.UnixListener); ok {
		unix.SetUnlinkOnClose(false)
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

func inspectSocketParent(parent string) (os.FileInfo, bool, error) {
	parentInfo, err := os.Lstat(parent)
	createdParent := false
	if os.IsNotExist(err) {
		if err := os.MkdirAll(parent, 0o700); err != nil {
			return nil, false, fmt.Errorf("create private socket directory: %w", err)
		}
		parentInfo, err = os.Lstat(parent)
		createdParent = true
	}
	if err != nil {
		return nil, false, fmt.Errorf("inspect socket directory: %w", err)
	}
	return parentInfo, createdParent, nil
}

func probeExistingEvidenceSocket(path string) error {
	probeCtx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	probe, dialErr := (&net.Dialer{}).DialContext(probeCtx, "unix", path)
	cancel()
	if dialErr == nil {
		_ = probe.Close()
		return fmt.Errorf("evidence socket is already active")
	}
	return fmt.Errorf("evidence socket path is occupied or stale: %w", dialErr)
}

func (l *cleanListener) removeOwnedSocket() error {
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

func secureCreatedSocketDirectory(parent string, parentInfo os.FileInfo, createdParent bool) error {
	if createdParent && parentInfo.Mode().Perm() != 0o700 {
		if err := os.Chmod(parent, 0o700); err != nil { //nolint:gosec // Private directory permissions are deliberately 0700.
			return fmt.Errorf("secure socket directory: %w", err)
		}
	}
	return nil
}
