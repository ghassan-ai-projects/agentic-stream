package app

import (
	"fmt"
	"maps"
	"slices"

	"github.com/ghassan-ai-projects/agentic-stream/internal/device"
	"github.com/ghassan-ai-projects/agentic-stream/internal/plantwire/internal/domain"
)

type commissioned struct {
	inventory domain.Inventory
	mapping   domain.Mapping
	digest    string
}

func openInventory(cfg Config) (commissioned, error) {
	kinds, site, err := readSources(cfg)
	if err != nil {
		return commissioned{}, err
	}
	inventory, err := domain.Commission(kinds, site.site.DeviceNames())
	if err != nil {
		return commissioned{}, fmt.Errorf("plantwire: commission %s: %w", cfg.KindsDir, err)
	}
	mapping, err := domain.NewMapping(site.site, inventory)
	if err != nil {
		return commissioned{}, fmt.Errorf("plantwire: site file %s: %w", cfg.SitePath, err)
	}
	return commissioned{inventory: inventory, mapping: mapping, digest: site.digest}, nil
}

func readSources(cfg Config) ([]domain.Kind, loadedSite, error) {
	kinds, err := commissionedKinds(cfg.KindsDir)
	if err != nil {
		return nil, loadedSite{}, err
	}
	site, err := loadSite(cfg.SitePath)
	if err != nil {
		return nil, loadedSite{}, err
	}
	return kinds, site, nil
}

func checkCatalogTargets(catalog *device.CapabilityCatalog, mapping domain.Mapping) error {
	if catalog == nil {
		return nil
	}
	for _, name := range slices.Sorted(maps.Keys(catalog.Routes)) {
		if err := checkSiteDevice(mapping, catalog.Routes[name].Target); err != nil {
			return fmt.Errorf("plant wire route %q: %w", name, err)
		}
	}
	return checkSafeStops(catalog, mapping)
}

func checkSafeStops(catalog *device.CapabilityCatalog, mapping domain.Mapping) error {
	for _, target := range slices.Sorted(maps.Keys(catalog.SafeStops)) {
		if err := checkSiteDevice(mapping, target); err != nil {
			return fmt.Errorf("plant wire safe stop %q: %w", target, err)
		}
	}
	return nil
}

func checkSiteDevice(mapping domain.Mapping, device string) error {
	if _, named := mapping.Directory().DeviceFor(device); !named {
		return fmt.Errorf("the target %q is no device of the site file", device)
	}
	return nil
}
