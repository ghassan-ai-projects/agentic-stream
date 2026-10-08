package transport

import (
	"crypto/tls"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func privateDir(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("", "as-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	return dir
}

func TestDialEpisodeWorkerSocketValidatesThePathAndDialsLazily(t *testing.T) {
	t.Parallel()
	if _, err := DialEpisodeWorkerSocketTLS(t.Context(), "relative.sock", nil); err == nil {
		t.Fatal("a relative worker socket path was accepted")
	}
	path := filepath.Join(privateDir(t), "worker.sock")
	for _, config := range []*tls.Config{nil, {MinVersion: tls.VersionTLS13}} {
		conn, err := DialEpisodeWorkerSocketTLS(t.Context(), path, config)
		if err != nil {
			t.Fatalf("dial: %v", err)
		}
		if err := conn.Close(); err != nil {
			t.Fatalf("close: %v", err)
		}
	}
}

func TestListenEvidenceSocketRefusesAnActiveOrUnsafePath(t *testing.T) {
	t.Parallel()
	dir := privateDir(t)
	active := filepath.Join(dir, "active.sock")
	listener, err := ListenEvidenceSocket(active)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = listener.Close() })
	if _, err := ListenEvidenceSocket(active); err == nil || !strings.Contains(err.Error(), "already active") {
		t.Fatalf("a second listener on an active socket was allowed: %v", err)
	}
	plain := filepath.Join(dir, "plain.sock")
	if err := os.WriteFile(plain, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := ListenEvidenceSocket(plain); err == nil || !strings.Contains(err.Error(), "unsafe existing socket") {
		t.Fatalf("a regular file was replaced by a socket: %v", err)
	}
}

func TestListenEvidenceSocketRefusesAStaleSocket(t *testing.T) {
	t.Parallel()
	path := filepath.Join(privateDir(t), "stale.sock")
	raw, err := (&net.ListenConfig{}).Listen(t.Context(), "unix", path)
	if err != nil {
		t.Fatal(err)
	}
	raw.(*net.UnixListener).SetUnlinkOnClose(false)
	_ = raw.Close()
	if _, err := ListenEvidenceSocket(path); err == nil || !strings.Contains(err.Error(), "occupied or stale") {
		t.Fatalf("a stale socket was reused: %v", err)
	}
}

func TestClosingTheListenerKeepsAReplacedSocket(t *testing.T) {
	t.Parallel()
	path := filepath.Join(privateDir(t), "owned.sock")
	listener, err := ListenEvidenceSocket(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := listener.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("cleanup removed a file the listener did not own: %v", err)
	}
}
