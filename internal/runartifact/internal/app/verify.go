package app

import (
	"fmt"

	domain "github.com/ghassan-ai-projects/agentic-stream/internal/runartifact/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/runartifact/internal/transport"
)

// Verify checks all checksums, expected files and JSON/JSONL syntax in a run
// directory, then the manifest bindings and ledger digests. It does not trust
// the manifest to define its own integrity: integrity is checked first.
func Verify(dir string, policy domain.PolicyDocuments) error {
	if dir == "" {
		return fmt.Errorf("run artifact directory is required")
	}
	if err := verifyIntegrity(dir); err != nil {
		return err
	}
	if err := verifyDocuments(dir); err != nil {
		return err
	}
	if err := verifyManifestBindings(dir, policy); err != nil {
		return err
	}
	return verifyLedgers(dir)
}

func verifyIntegrity(dir string) error {
	expected, err := readChecksumIndex(dir)
	if err != nil {
		return err
	}
	if err := verifyChecksums(dir, expected); err != nil {
		return err
	}
	entries, err := transport.Entries(dir)
	if err != nil {
		return err
	}
	return domain.RequireOnlyIndexedFiles(entries, expected)
}

// readChecksumIndex reads the directory's checksum index and requires it to
// list every file an artifact has.
func readChecksumIndex(dir string) (map[string]string, error) {
	if err := transport.RequireDirectory(dir); err != nil {
		return nil, err
	}
	data, err := transport.Read(dir, domain.ChecksumsFile)
	if err != nil {
		return nil, fmt.Errorf("read run artifact checksums: %w", err)
	}
	expected, err := domain.ParseChecksumIndex(data)
	if err != nil {
		return nil, err //nolint:wrapcheck // The domain rule's message is the operator-facing text.
	}
	return expected, domain.RequireExpectedFiles(expected) //nolint:wrapcheck // The domain rule's message is the operator-facing text.
}

func verifyChecksums(dir string, expected map[string]string) error {
	for name, wanted := range expected {
		data, err := transport.Read(dir, name)
		if err != nil {
			return fmt.Errorf("read %s: %w", name, err)
		}
		if err := domain.VerifyChecksum(name, data, wanted); err != nil {
			return err
		}
	}
	return nil
}

func verifyDocuments(dir string) error {
	for _, name := range domain.DocumentFiles() {
		data, err := transport.Read(dir, name)
		if err != nil {
			return fmt.Errorf("read %s: %w", name, err)
		}
		if err := domain.VerifyDocument(data); err != nil {
			return fmt.Errorf("verify %s: %w", name, err)
		}
	}
	return nil
}

func readManifest(dir string) (domain.Manifest, error) {
	data, err := transport.Read(dir, domain.FileManifest)
	if err != nil {
		return domain.Manifest{}, fmt.Errorf("read manifest.json: %w", err)
	}
	return domain.DecodeManifest(data) //nolint:wrapcheck // The domain rule's message is the operator-facing text.
}

func verifyManifestBindings(dir string, policy domain.PolicyDocuments) error {
	manifest, err := readManifest(dir)
	if err != nil {
		return err
	}
	specData, policyData, err := readBoundDefinitions(dir)
	if err != nil {
		return err
	}
	if err := policy.VerifyBindings(manifest, specData, policyData); err != nil {
		return fmt.Errorf("verify manifest bindings: %w", err)
	}
	return nil
}

func readBoundDefinitions(dir string) ([]byte, []byte, error) {
	specData, err := transport.Read(dir, domain.FileSpec)
	if err != nil {
		return nil, nil, fmt.Errorf("read %s: %w", domain.FileSpec, err)
	}
	policyData, err := transport.Read(dir, domain.FilePolicy)
	if err != nil {
		return nil, nil, fmt.Errorf("read %s: %w", domain.FilePolicy, err)
	}
	return specData, policyData, nil
}

func verifyLedgers(dir string) error {
	for _, name := range domain.LedgerFiles() {
		data, err := transport.Read(dir, name)
		if err != nil {
			return fmt.Errorf("read %s: %w", name, err)
		}
		if err := domain.VerifyLedger(name, data); err != nil {
			return err
		}
	}
	return nil
}
