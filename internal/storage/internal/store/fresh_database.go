package store

import (
	"fmt"
	"os"

	"github.com/ghassan-ai-projects/agentic-stream/internal/storage/internal/domain"
)

// reserveFreshDatabase owns the replay reservation until the caller opens
// SQLite. Failure releases only the reservation, never existing files.
func reserveFreshDatabase(path string) (string, error) {
	reservationPath := domain.ReservationPath(path)
	if err := os.Mkdir(reservationPath, 0o700); err != nil {
		return "", fmt.Errorf("reserve replay directory: %w", err)
	}
	if err := checkFreshSidecars(path); err != nil {
		_ = os.Remove(reservationPath)
		return "", err
	}
	if err := createFreshDatabaseFile(path); err != nil {
		_ = os.Remove(reservationPath)
		return "", err
	}
	return reservationPath, nil
}

func checkFreshSidecars(path string) error {
	for _, sidecar := range domain.FreshSidecars(path) {
		if _, err := os.Lstat(sidecar); err == nil {
			return fmt.Errorf("fresh database sidecar already exists: %s", sidecar)
		} else if !os.IsNotExist(err) {
			return fmt.Errorf("inspect fresh database sidecar: %w", err)
		}
	}
	return nil
}

func createFreshDatabaseFile(path string) error {
	file, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return fmt.Errorf("reserve fresh database path: %w", err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("close reserved database path: %w", err)
	}
	return nil
}
