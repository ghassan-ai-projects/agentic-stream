package transport

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"os"
)

func loadWorkerTLS(caPath, certPath, keyPath, serverName string) (*tls.Config, error) {
	if caPath == "" && certPath == "" && keyPath == "" {
		return nil, nil
	}
	pool, err := loadWorkerCertPool(caPath)
	if err != nil {
		return nil, err
	}
	certificate, err := tls.LoadX509KeyPair(certPath, keyPath)
	if err != nil {
		return nil, fmt.Errorf("load worker client certificate: %w", err)
	}
	return &tls.Config{RootCAs: pool, Certificates: []tls.Certificate{certificate}, MinVersion: tls.VersionTLS13, ServerName: serverName}, nil
}

func loadWorkerCertPool(caPath string) (*x509.CertPool, error) {
	caPEM, err := os.ReadFile(caPath)
	if err != nil {
		return nil, fmt.Errorf("read worker CA: %w", err)
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(caPEM) {
		return nil, fmt.Errorf("worker CA contains no certificates")
	}
	return pool, nil
}
