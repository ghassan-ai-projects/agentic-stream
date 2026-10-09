package domain

import "testing"

func TestValidateWorkerOptions(t *testing.T) {
	t.Parallel()
	validKey := "000102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f"
	tests := []struct {
		name    string
		config  WorkerOptions
		wantErr string
	}{
		{name: "native defaults", config: WorkerOptions{}},
		{name: "evidence requires worker", config: WorkerOptions{EvidenceSocket: "/tmp/evidence.sock", EvidenceKey: validKey}, wantErr: "--evidence-socket requires --worker-socket"},
		{name: "evidence key requires socket", config: WorkerOptions{EvidenceKey: validKey}, wantErr: "--evidence-key requires --evidence-socket"},
		{name: "evidence key is bounded", config: WorkerOptions{WorkerSocket: "/tmp/worker.sock", WorkerName: "worker", EvidenceSocket: "/tmp/evidence.sock", EvidenceKey: "00"}, wantErr: "--evidence-key must be at least 32 bytes of hex"},
		{name: "worker name is required", config: WorkerOptions{WorkerSocket: "/tmp/worker.sock"}, wantErr: "--worker-name is required with --worker-socket"},
		{name: "mTLS is all or nothing", config: WorkerOptions{WorkerSocket: "/tmp/worker.sock", WorkerName: "worker", WorkerCA: "ca.pem"}, wantErr: "--worker-ca, --worker-cert, and --worker-key are required together"},
		{name: "mTLS requires worker", config: WorkerOptions{WorkerCA: "ca.pem", WorkerCert: "cert.pem", WorkerKey: "key.pem", WorkerServerName: "worker.local"}, wantErr: "worker TLS flags require --worker-socket"},
		{name: "mTLS server name is required", config: WorkerOptions{WorkerSocket: "/tmp/worker.sock", WorkerName: "worker", WorkerCA: "ca.pem", WorkerCert: "cert.pem", WorkerKey: "key.pem"}, wantErr: "--worker-server-name is required with mTLS"},
		{name: "server name requires mTLS", config: WorkerOptions{WorkerSocket: "/tmp/worker.sock", WorkerName: "worker", WorkerServerName: "worker.local"}, wantErr: "--worker-server-name requires mTLS"},
		{name: "model name is required", config: WorkerOptions{ModelEndpoint: "http://model"}, wantErr: "--model-name is required with --model-endpoint"},
		{name: "complete worker and evidence", config: WorkerOptions{WorkerSocket: "/tmp/worker.sock", WorkerName: "worker", WorkerCA: "ca.pem", WorkerCert: "cert.pem", WorkerKey: "key.pem", WorkerServerName: "worker.local", EvidenceSocket: "/tmp/evidence.sock", EvidenceKey: validKey}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := ValidateWorkerOptions(tt.config)
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("ValidateWorkerOptions() error = %v", err)
				}
				return
			}
			if err == nil || err.Error() != tt.wantErr {
				t.Fatalf("ValidateWorkerOptions() error = %v, want %q", err, tt.wantErr)
			}
		})
	}
}

func TestDecodeEvidenceKeyAcceptsOnlyHexOfAtLeastThirtyTwoBytes(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name, key string
		wantLen   int
		wantErr   bool
	}{
		{"thirty-two bytes", "000102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f", 32, false},
		{"longer keys are kept whole", "000102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f20", 33, false},
		{"too short", "0001", 0, true},
		{"not hex", "not-hex", 0, true},
		{"empty", "", 0, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			key, err := DecodeEvidenceKey(tt.key)
			if (err != nil) != tt.wantErr || len(key) != tt.wantLen {
				t.Fatalf("DecodeEvidenceKey(%q) = %d bytes, %v; want %d bytes, error=%v", tt.key, len(key), err, tt.wantLen, tt.wantErr)
			}
		})
	}
}
