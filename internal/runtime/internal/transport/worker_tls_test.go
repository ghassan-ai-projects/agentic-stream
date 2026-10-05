package transport

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestWorkerTLSLoadsBoundedMutualTLSConfiguration(t *testing.T) {
	t.Parallel()
	public, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	cert := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "worker"}, NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour), IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth}}
	der, err := x509.CreateCertificate(rand.Reader, cert, cert, public, private)
	if err != nil {
		t.Fatal(err)
	}
	key, err := x509.MarshalPKCS8PrivateKey(private)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	certPath := filepath.Join(dir, "cert.pem")
	keyPath := filepath.Join(dir, "key.pem")
	if err = os.WriteFile(certPath, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0o600); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(keyPath, pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: key}), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := loadWorkerTLS(certPath, certPath, keyPath, "worker.local")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.MinVersion != tls.VersionTLS13 || cfg.ServerName != "worker.local" || len(cfg.Certificates) != 1 || cfg.RootCAs == nil {
		t.Fatalf("TLS configuration=%+v", cfg)
	}
	if _, err = loadWorkerTLS(certPath, certPath, "/missing/key", "worker.local"); err == nil || !strings.Contains(err.Error(), "load worker client certificate") {
		t.Fatalf("key error=%v", err)
	}
}

func TestWorkerTLSRejectsInvalidCAAndAllowsUnconfiguredTransport(t *testing.T) {
	t.Parallel()
	if cfg, err := loadWorkerTLS("", "", "", ""); err != nil || cfg != nil {
		t.Fatalf("unconfigured TLS: %v %v", cfg, err)
	}
	if _, err := loadWorkerCertPool("/missing/ca"); err == nil || !strings.Contains(err.Error(), "read worker CA") {
		t.Fatalf("missing CA: %v", err)
	}
	invalid := filepath.Join(t.TempDir(), "ca.pem")
	if err := os.WriteFile(invalid, []byte("invalid"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := loadWorkerTLS(invalid, "cert", "key", "worker"); err == nil || err.Error() != "worker CA contains no certificates" {
		t.Fatalf("invalid CA: %v", err)
	}
}
