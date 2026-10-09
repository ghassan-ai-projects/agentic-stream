package transport

import (
	"crypto/tls"
	"path/filepath"
	"strings"
	"testing"
)

func TestDialEpisodeWorkerSocketValidatesThePathAndDialsLazily(t *testing.T) {
	t.Parallel()
	if _, err := DialEpisodeWorkerSocketTLS(t.Context(), "relative.sock", nil); err == nil || !strings.Contains(err.Error(), "clean absolute Unix path") {
		t.Fatalf("a relative worker socket path was accepted: %v", err)
	}
	path := filepath.Join(privateDir(t), "worker.sock")
	for name, config := range map[string]*tls.Config{"plain": nil, "tls": {MinVersion: tls.VersionTLS13}} {
		conn, err := DialEpisodeWorkerSocketTLS(t.Context(), path, config)
		if err != nil {
			t.Fatalf("%s dial: %v", name, err)
		}
		if err := conn.Close(); err != nil {
			t.Fatalf("%s close: %v", name, err)
		}
	}
}
