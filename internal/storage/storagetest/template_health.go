package storagetest

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
)

func readSoundTemplate(path string) ([]byte, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read template database: %w", err)
	}
	if len(content) == 0 {
		return nil, fmt.Errorf("template database %s is empty", path)
	}
	if err := quickCheck(path); err != nil {
		return nil, err
	}
	return content, nil
}

func quickCheck(path string) error {
	source := url.URL{Scheme: "file", Path: path, RawQuery: "mode=ro&immutable=1"}
	db, err := sql.Open("sqlite", source.String())
	if err != nil {
		return fmt.Errorf("open template database %s: %w", path, err)
	}
	defer func() { _ = db.Close() }()
	var verdict string
	if err := db.QueryRowContext(context.Background(), "PRAGMA quick_check").Scan(&verdict); err != nil {
		return fmt.Errorf("check template database %s: %w", path, err)
	}
	if verdict != "ok" {
		return fmt.Errorf("template database %s failed its quick check: %s", path, verdict)
	}
	return nil
}

func removeOtherTemplates(directory, keep string) {
	others, err := filepath.Glob(filepath.Join(directory, "template-*.db"))
	if err != nil {
		return
	}
	for _, other := range others {
		if other != keep {
			_ = os.Remove(other)
		}
	}
}
