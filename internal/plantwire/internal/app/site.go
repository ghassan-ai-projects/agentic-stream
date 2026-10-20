package app

import (
	"fmt"
	"os"

	"github.com/ghassan-ai-projects/agentic-stream/internal/canonicaljson"
	"github.com/ghassan-ai-projects/agentic-stream/internal/plantwire/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/plantwire/internal/wire"
)

type loadedSite struct {
	site   domain.Site
	digest string
}

func loadSite(path string) (loadedSite, error) {
	if path == "" {
		return loadedSite{site: domain.Site{}}, nil
	}
	raw, site, err := readSite(path)
	if err != nil {
		return loadedSite{}, err
	}
	return loadedSite{site: site, digest: canonicaljson.ContentDigest(raw)}, nil
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
