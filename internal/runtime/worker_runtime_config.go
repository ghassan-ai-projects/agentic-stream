package runtime

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"fmt"
	"os"
)

// ValidateWorkerRuntimeConfig rejects incomplete worker/evidence combinations
// before sockets or credentials are opened.
func ValidateWorkerRuntimeConfig(cfg WorkerRuntimeConfig) error {
	if err := validateEvidenceFlags(cfg); err != nil {
		return err
	}
	if cfg.WorkerSocket != "" && cfg.WorkerName == "" {
		return fmt.Errorf("--worker-name is required with --worker-socket")
	}
	if err := validateWorkerTLSFlags(cfg); err != nil {
		return err
	}
	if cfg.ModelEndpoint != "" && cfg.ModelName == "" {
		return fmt.Errorf("--model-name is required with --model-endpoint")
	}
	return nil
}

func validateEvidenceFlags(cfg WorkerRuntimeConfig) error {
	if cfg.EvidenceSocket != "" && cfg.WorkerSocket == "" {
		return fmt.Errorf("--evidence-socket requires --worker-socket")
	}
	if cfg.EvidenceKey != "" && cfg.EvidenceSocket == "" {
		return fmt.Errorf("--evidence-key requires --evidence-socket")
	}
	if cfg.EvidenceSocket != "" {
		if _, err := decodeEvidenceKey(cfg.EvidenceKey); err != nil {
			return err
		}
	}
	return nil
}

func validateWorkerTLSFlags(cfg WorkerRuntimeConfig) error {
	if cfg.WorkerSocket == "" && (cfg.WorkerCA != "" || cfg.WorkerCert != "" || cfg.WorkerKey != "" || cfg.WorkerServerName != "") {
		return fmt.Errorf("worker TLS flags require --worker-socket")
	}
	if (cfg.WorkerCA != "") != (cfg.WorkerCert != "") || (cfg.WorkerCA != "") != (cfg.WorkerKey != "") {
		return fmt.Errorf("--worker-ca, --worker-cert, and --worker-key are required together")
	}
	if cfg.WorkerCA == "" && cfg.WorkerServerName != "" {
		return fmt.Errorf("--worker-server-name requires mTLS")
	}
	if cfg.WorkerCA != "" && cfg.WorkerServerName == "" {
		return fmt.Errorf("--worker-server-name is required with mTLS")
	}
	return nil
}

// decodeEvidenceKey decodes the evidence capability HMAC key, which must be
// at least 32 bytes.
func decodeEvidenceKey(key string) ([]byte, error) {
	secret, err := hex.DecodeString(key)
	if err != nil || len(secret) < 32 {
		return nil, fmt.Errorf("--evidence-key must be at least 32 bytes of hex")
	}
	return secret, nil
}

func loadWorkerTLS(caPath, certPath, keyPath, serverName string) (*tls.Config, error) {
	if caPath == "" && certPath == "" && keyPath == "" {
		return nil, nil
	}
	caPEM, err := os.ReadFile(caPath)
	if err != nil {
		return nil, fmt.Errorf("read worker CA: %w", err)
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(caPEM) {
		return nil, fmt.Errorf("worker CA contains no certificates")
	}
	certificate, err := tls.LoadX509KeyPair(certPath, keyPath)
	if err != nil {
		return nil, fmt.Errorf("load worker client certificate: %w", err)
	}
	return &tls.Config{RootCAs: pool, Certificates: []tls.Certificate{certificate}, MinVersion: tls.VersionTLS13, ServerName: serverName}, nil
}
