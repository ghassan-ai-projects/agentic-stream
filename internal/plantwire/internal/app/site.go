package app

import (
	"fmt"
	"os"

	"github.com/ghassan-ai-projects/agentic-stream/internal/canonicaljson"
	"github.com/ghassan-ai-projects/agentic-stream/internal/plantwire/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/plantwire/internal/wire"
)

type loadedSite struct {
	mapping domain.Mapping
	digest  string
}

func loadSite(path string, inventory domain.Inventory) (loadedSite, error) {
	if path == "" {
		return loadedSite{}, nil
	}
	raw, site, err := readSite(path)
	if err != nil {
		return loadedSite{}, err
	}
	mapping, err := domain.NewMapping(site, inventory)
	if err != nil {
		return loadedSite{}, fmt.Errorf("plantwire: site file %s: %w", path, err)
	}
	return loadedSite{mapping: mapping, digest: canonicaljson.ContentDigest(raw)}, nil
}

func readSite(path string) ([]byte, domain.Site, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, domain.Site{}, fmt.Errorf("plantwire: read the site file: %w", err)
	}
	site, err := wire.ParseSiteFile(raw)
	if err != nil {
		return nil, domain.Site{}, fmt.Errorf("plantwire: site file %s: %w", path, err)
	}
	return raw, site, nil
}
