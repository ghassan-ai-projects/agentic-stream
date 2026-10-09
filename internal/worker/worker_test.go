package worker_test

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"math/big"
	"path/filepath"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/protobuf/types/known/durationpb"

	"github.com/ghassan-ai-projects/agentic-stream/internal/testsupport/workerfake"
	"github.com/ghassan-ai-projects/agentic-stream/internal/worker"
	runtimev1 "github.com/ghassan-ai-projects/agentic-stream/proto/agenticstream/runtime/v1"
)

func TestFacadeExposesTheProtocolRules(t *testing.T) {
	t.Parallel()
	if worker.ProtocolVersion != "1.0" || worker.ContractVersion != "1.0" || worker.EvidenceToolsFeature != "evidence_tools.v1" ||
		worker.DefaultMaxEvents != 4096 || worker.DefaultMaxStreamBytes != 16<<20 {
		t.Fatal("the facade must expose the frozen protocol identity and stream limits")
	}
	if err := worker.ValidateBudget(&runtimev1.EpisodeBudget{WallTime: durationpb.New(time.Second)}); err != nil {
		t.Errorf("ValidateBudget(wall time) = %v", err)
	}
	if err := worker.ValidateBudget(nil); err == nil {
		t.Error("a budget without a wall time was accepted")
	}
	if err := worker.ValidateEvidenceSocketPath("/tmp/e.sock"); err != nil {
		t.Errorf("ValidateEvidenceSocketPath(clean path) = %v", err)
	}
	if err := worker.ValidateEvidenceSocketPath("relative"); err == nil {
		t.Error("a relative socket path was accepted")
	}
}

func TestWorkerAnswersOverThePrivateSocketItIsDialledOn(t *testing.T) {
	t.Parallel()
	path := filepath.Join(workerfake.SocketDir(t), "worker.sock")
	serveWorker(t, path, nil)
	conn, err := worker.DialEpisodeWorkerSocketTLS(t.Context(), path, nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	if _, err := handshake(t.Context(), conn); err != nil {
		t.Fatalf("handshake over a plain socket: %v", err)
	}
}

func TestWorkerSocketWithTLSAdmitsOnlyAClientThatTrustsTheServer(t *testing.T) {
	t.Parallel()
	certificate, pool := selfSignedServer(t)
	path := filepath.Join(workerfake.SocketDir(t), "worker.sock")
	serveWorker(t, path, grpc.Creds(credentials.NewServerTLSFromCert(&certificate)))
	trusting := &tls.Config{MinVersion: tls.VersionTLS13, RootCAs: pool, ServerName: "localhost"}
	tests := []struct {
		name    string
		config  *tls.Config
		wantErr bool
	}{
		{"client trusting the server", trusting, false},
		{"client without TLS", nil, true},
		{"client trusting nobody", &tls.Config{MinVersion: tls.VersionTLS13, ServerName: "localhost"}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			conn, err := worker.DialEpisodeWorkerSocketTLS(t.Context(), path, tt.config)
			if err != nil {
				t.Fatalf("dial: %v", err)
			}
			t.Cleanup(func() { _ = conn.Close() })
			ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
			defer cancel()
			if _, err := handshake(ctx, conn); (err != nil) != tt.wantErr {
				t.Fatalf("handshake error = %v, want error=%v", err, tt.wantErr)
			}
		})
	}
}

func serveWorker(t *testing.T, path string, option grpc.ServerOption) {
	t.Helper()
	listener, err := worker.ListenEvidenceSocket(path)
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	var options []grpc.ServerOption
	if option != nil {
		options = append(options, option)
	}
	server := grpc.NewServer(options...)
	runtimev1.RegisterEpisodeWorkerServer(server, &workerfake.Server{WorkerName: "worker-1", WorkerVersion: "test"})
	go func() { _ = server.Serve(listener) }()
	t.Cleanup(server.Stop)
}

func handshake(ctx context.Context, conn *grpc.ClientConn) (*runtimev1.HandshakeResponse, error) {
	return runtimev1.NewEpisodeWorkerClient(conn).Handshake(ctx, &runtimev1.HandshakeRequest{ //nolint:wrapcheck // The test inspects the transport's own error.
		ProtocolVersion: worker.ProtocolVersion, ContractVersion: worker.ContractVersion,
		WorkerId: "worker-1", RuntimeInstanceId: "runtime-1", NonInteractive: true,
	})
}

func selfSignedServer(t *testing.T) (tls.Certificate, *x509.CertPool) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	template := &x509.Certificate{
		SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "localhost"}, DNSNames: []string{"localhost"},
		NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour),
		KeyUsage: x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true, IsCA: true,
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	leaf, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	pool := x509.NewCertPool()
	pool.AddCert(leaf)
	return tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key, Leaf: leaf}, pool
}
