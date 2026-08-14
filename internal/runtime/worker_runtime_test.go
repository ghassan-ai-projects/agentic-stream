package runtime

import "testing"

func TestValidateWorkerRuntimeConfig(t *testing.T) {
	validKey := "000102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f"
	tests := []struct {
		name    string
		config  WorkerRuntimeConfig
		wantErr string
	}{
		{name: "native defaults", config: WorkerRuntimeConfig{}},
		{name: "evidence requires worker", config: WorkerRuntimeConfig{EvidenceSocket: "/tmp/evidence.sock", EvidenceKey: validKey}, wantErr: "--evidence-socket requires --worker-socket"},
		{name: "evidence key requires socket", config: WorkerRuntimeConfig{EvidenceKey: validKey}, wantErr: "--evidence-key requires --evidence-socket"},
		{name: "evidence key is bounded", config: WorkerRuntimeConfig{WorkerSocket: "/tmp/worker.sock", WorkerName: "worker", EvidenceSocket: "/tmp/evidence.sock", EvidenceKey: "00"}, wantErr: "--evidence-key must be at least 32 bytes of hex"},
		{name: "worker name is required", config: WorkerRuntimeConfig{WorkerSocket: "/tmp/worker.sock"}, wantErr: "--worker-name is required with --worker-socket"},
		{name: "mTLS is all or nothing", config: WorkerRuntimeConfig{WorkerSocket: "/tmp/worker.sock", WorkerName: "worker", WorkerCA: "ca.pem"}, wantErr: "--worker-ca, --worker-cert, and --worker-key are required together"},
		{name: "mTLS requires worker", config: WorkerRuntimeConfig{WorkerCA: "ca.pem", WorkerCert: "cert.pem", WorkerKey: "key.pem", WorkerServerName: "worker.local"}, wantErr: "worker TLS flags require --worker-socket"},
		{name: "mTLS server name is required", config: WorkerRuntimeConfig{WorkerSocket: "/tmp/worker.sock", WorkerName: "worker", WorkerCA: "ca.pem", WorkerCert: "cert.pem", WorkerKey: "key.pem"}, wantErr: "--worker-server-name is required with mTLS"},
		{name: "server name requires mTLS", config: WorkerRuntimeConfig{WorkerSocket: "/tmp/worker.sock", WorkerName: "worker", WorkerServerName: "worker.local"}, wantErr: "--worker-server-name requires mTLS"},
		{name: "model name is required", config: WorkerRuntimeConfig{ModelEndpoint: "http://model"}, wantErr: "--model-name is required with --model-endpoint"},
		{name: "complete worker and evidence", config: WorkerRuntimeConfig{WorkerSocket: "/tmp/worker.sock", WorkerName: "worker", WorkerCA: "ca.pem", WorkerCert: "cert.pem", WorkerKey: "key.pem", WorkerServerName: "worker.local", EvidenceSocket: "/tmp/evidence.sock", EvidenceKey: validKey}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateWorkerRuntimeConfig(tt.config)
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("ValidateWorkerRuntimeConfig() error = %v", err)
				}
				return
			}
			if err == nil || err.Error() != tt.wantErr {
				t.Fatalf("ValidateWorkerRuntimeConfig() error = %v, want %q", err, tt.wantErr)
			}
		})
	}
}
