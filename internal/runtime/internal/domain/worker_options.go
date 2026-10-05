package domain

import (
	"encoding/hex"
	"fmt"
)

// WorkerOptions contains the complete validated composition for native
// or process-isolated episode execution. The same configuration is used by
// run-live and serve so their worker and evidence boundaries cannot drift.
type WorkerOptions struct {
	RuntimeEpoch     string
	WorkerSocket     string
	WorkerName       string
	WorkerCA         string
	WorkerCert       string
	WorkerKey        string
	WorkerServerName string
	EvidenceSocket   string
	EvidenceKey      string
	ModelEndpoint    string
	ModelName        string
}

// ValidateWorkerOptions rejects incomplete worker/evidence combinations
// before sockets or credentials are opened.
func ValidateWorkerOptions(cfg WorkerOptions) error {
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

func validateEvidenceFlags(cfg WorkerOptions) error {
	if cfg.EvidenceSocket != "" && cfg.WorkerSocket == "" {
		return fmt.Errorf("--evidence-socket requires --worker-socket")
	}
	if cfg.EvidenceKey != "" && cfg.EvidenceSocket == "" {
		return fmt.Errorf("--evidence-key requires --evidence-socket")
	}
	if cfg.EvidenceSocket != "" {
		if _, err := DecodeEvidenceKey(cfg.EvidenceKey); err != nil {
			return err
		}
	}
	return nil
}

func validateWorkerTLSFlags(cfg WorkerOptions) error {
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

// DecodeEvidenceKey decodes the evidence capability HMAC key, which must be
// at least 32 bytes.
func DecodeEvidenceKey(key string) ([]byte, error) {
	secret, err := hex.DecodeString(key)
	if err != nil || len(secret) < 32 {
		return nil, fmt.Errorf("--evidence-key must be at least 32 bytes of hex")
	}
	return secret, nil
}
