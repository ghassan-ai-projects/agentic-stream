package domain

import (
	"errors"
	"fmt"
	"maps"
	"slices"

	"github.com/ghassan-ai-projects/agentic-stream/internal/contractsv1"
)

var (
	ErrSiteKindNotCommissioned = errors.New("plantwire: the site file names a kind no kind file commissions")
	ErrSiteMetricNotDeclared   = errors.New("plantwire: the site file maps a metric its kind does not declare")
	ErrSiteRuleTwice           = errors.New("plantwire: the site file has two rules for one reading or heartbeat")
	ErrSiteDeviceWithoutEntity = errors.New("plantwire: a site device of a kind that gives events names no entity")
	ErrSiteQualityLikeGood     = errors.New("plantwire: the site file maps a wire quality other than good to the event quality of a good sample")
)

type Site struct {
	Version    int                   `json:"version"`
	Source     string                `json:"source"`
	Devices    map[string]SiteDevice `json:"devices"`
	Readings   []Reading             `json:"readings"`
	Heartbeats []HeartbeatRule       `json:"heartbeats"`
	Quality    map[string]string     `json:"quality"`
}

type SiteDevice struct {
	Kind   string                 `json:"kind"`
	Entity *contractsv1.EntityRef `json:"entity"`
}

type Emitted struct {
	EventType     string `json:"event_type"`
	SchemaVersion string `json:"schema_version"`
}

type Reading struct {
	Kind   string `json:"kind"`
	Metric string `json:"metric"`
	Field  string `json:"field"`
	Emitted
}

type HeartbeatRule struct {
	Kind         string `json:"kind"`
	EveryMS      int64  `json:"every_ms"`
	StaleAfterMS int64  `json:"stale_after_ms"`
	Emitted
}

type readingKey struct{ kind, metric string }

type Mapping struct {
	source     string
	directory  Directory
	readings   map[readingKey]Reading
	heartbeats map[string]HeartbeatRule
	quality    map[string]string
}

func NewMapping(site Site, inventory Inventory) (Mapping, error) {
	if err := site.check(inventory); err != nil {
		return Mapping{}, err
	}
	mapping := Mapping{source: site.Source, directory: Directory{devices: site.Devices}, quality: site.Quality,
		readings: map[readingKey]Reading{}, heartbeats: map[string]HeartbeatRule{}}
	for _, reading := range site.Readings {
		mapping.readings[readingKey{reading.Kind, reading.Metric}] = reading
	}
	for _, rule := range site.Heartbeats {
		mapping.heartbeats[rule.Kind] = rule
	}
	return mapping, nil
}

func (m Mapping) reading(kind, metric string) (Reading, bool) {
	reading, mapped := m.readings[readingKey{kind, metric}]
	return reading, mapped
}

func (s Site) check(inventory Inventory) error {
	if err := s.checkQuality(); err != nil {
		return err
	}
	for _, check := range []func(Inventory) error{s.checkDevices, s.checkReadings, s.checkHeartbeats} {
		if err := check(inventory); err != nil {
			return err
		}
	}
	return nil
}

func (s Site) checkQuality() error {
	for _, code := range slices.Sorted(maps.Keys(s.Quality)) {
		if code != qualityGood && s.Quality[code] == s.Quality[qualityGood] {
			return fmt.Errorf("%w: %s maps to %q", ErrSiteQualityLikeGood, code, s.Quality[code])
		}
	}
	return nil
}

func (s Site) checkDevices(inventory Inventory) error {
	for _, name := range slices.Sorted(maps.Keys(s.Devices)) {
		device := s.Devices[name]
		if !inventory.Commissioned(device.Kind) {
			return fmt.Errorf("%w: device %q of kind %q", ErrSiteKindNotCommissioned, name, device.Kind)
		}
		if device.Entity == nil && s.emitsFor(device.Kind) {
			return fmt.Errorf("%w: device %q of kind %q", ErrSiteDeviceWithoutEntity, name, device.Kind)
		}
	}
	return nil
}

func (s Site) checkReadings(inventory Inventory) error {
	seen := map[readingKey]bool{}
	for _, reading := range s.Readings {
		key := readingKey{reading.Kind, reading.Metric}
		if seen[key] {
			return fmt.Errorf("%w: metric %q of kind %q", ErrSiteRuleTwice, reading.Metric, reading.Kind)
		}
		if !inventory.Declares(reading.Kind, reading.Metric) {
			return fmt.Errorf("%w: metric %q of kind %q", ErrSiteMetricNotDeclared, reading.Metric, reading.Kind)
		}
		seen[key] = true
	}
	return nil
}

func (s Site) checkHeartbeats(inventory Inventory) error {
	seen := map[string]bool{}
	for _, rule := range s.Heartbeats {
		if seen[rule.Kind] {
			return fmt.Errorf("%w: heartbeat of kind %q", ErrSiteRuleTwice, rule.Kind)
		}
		if !inventory.Commissioned(rule.Kind) {
			return fmt.Errorf("%w: heartbeat of kind %q", ErrSiteKindNotCommissioned, rule.Kind)
		}
		seen[rule.Kind] = true
	}
	return nil
}

func (s Site) emitsFor(kind string) bool {
	return slices.ContainsFunc(s.Readings, func(r Reading) bool { return r.Kind == kind }) ||
		slices.ContainsFunc(s.Heartbeats, func(h HeartbeatRule) bool { return h.Kind == kind })
}

func (s Site) DeviceNames() []string {
	return slices.Sorted(maps.Keys(s.Devices))
}

type Directory struct{ devices map[string]SiteDevice }

func (d Directory) EntityOf(device string) (contractsv1.EntityRef, bool) {
	entry, named := d.devices[device]
	if !named || entry.Entity == nil {
		return contractsv1.EntityRef{}, false
	}
	return *entry.Entity, true
}

func (d Directory) KindOf(device string) (string, bool) {
	entry, named := d.devices[device]
	return entry.Kind, named
}

func (d Directory) DeviceFor(target string) (string, bool) {
	_, named := d.devices[target]
	return target, named
}

func (d Directory) KindOfEntity(entityID string) (string, bool) {
	for _, device := range slices.Sorted(maps.Keys(d.devices)) {
		if entry := d.devices[device]; entry.Entity != nil && entry.Entity.ID == entityID {
			return entry.Kind, true
		}
	}
	return "", false
}

func (m Mapping) Directory() Directory { return m.directory }
