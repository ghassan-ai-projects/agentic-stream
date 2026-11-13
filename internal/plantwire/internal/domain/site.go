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
	ErrSiteReadingDeviceAbsent = errors.New("plantwire: a site reading names a device the site file does not list")
	ErrSiteReadingDeviceKind   = errors.New("plantwire: a site reading names a device of another kind than the reading's")
	ErrSiteRuleShared          = errors.New("plantwire: a kind-level rule covers several devices of one entity, whose boots would mix in one event type")
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
	Device string `json:"device,omitempty"`
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

type readingKey struct{ device, kind, metric string }

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
		mapping.readings[reading.key()] = reading
	}
	for _, rule := range site.Heartbeats {
		mapping.heartbeats[rule.Kind] = rule
	}
	return mapping, nil
}

func (m Mapping) reading(device, kind, metric string) (Reading, bool) {
	if reading, mapped := m.readings[readingKey{device, kind, metric}]; mapped {
		return reading, true
	}
	reading, mapped := m.readings[readingKey{"", kind, metric}]
	return reading, mapped
}

func (r Reading) key() readingKey { return readingKey{r.Device, r.Kind, r.Metric} }

func (r Reading) described() string {
	if r.Device == "" {
		return fmt.Sprintf("metric %q of kind %q", r.Metric, r.Kind)
	}
	return fmt.Sprintf("metric %q of kind %q on device %q", r.Metric, r.Kind, r.Device)
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
		if seen[reading.key()] {
			return fmt.Errorf("%w: %s", ErrSiteRuleTwice, reading.described())
		}
		if err := s.checkReading(reading, inventory); err != nil {
			return err
		}
		seen[reading.key()] = true
	}
	return nil
}

func (s Site) checkReading(reading Reading, inventory Inventory) error {
	if !inventory.Declares(reading.Kind, reading.Metric) {
		return fmt.Errorf("%w: %s", ErrSiteMetricNotDeclared, reading.described())
	}
	device, listed := s.Devices[reading.Device]
	switch {
	case reading.Device == "":
		return s.checkKindReadingUnshared(reading)
	case !listed:
		return fmt.Errorf("%w: %s", ErrSiteReadingDeviceAbsent, reading.described())
	case device.Kind != reading.Kind:
		return fmt.Errorf("%w: %s, the device is of kind %q", ErrSiteReadingDeviceKind, reading.described(), device.Kind)
	}
	return nil
}

func (s Site) checkKindReadingUnshared(reading Reading) error {
	ownRule := func(device string) bool {
		return slices.ContainsFunc(s.Readings, func(r Reading) bool {
			return r.Device == device && r.Kind == reading.Kind && r.Metric == reading.Metric
		})
	}
	if entity, shared := s.sharedEntity(reading.Kind, ownRule); shared {
		return fmt.Errorf("%w: %s on entity %q; give each of its devices its own reading (\"device\")", ErrSiteRuleShared, reading.described(), entity)
	}
	return nil
}

func (s Site) sharedEntity(kind string, ownRule func(device string) bool) (string, bool) {
	devicesOn := map[string]int{}
	for _, name := range s.DeviceNames() {
		device := s.Devices[name]
		if device.Kind != kind || device.Entity == nil || ownRule(name) {
			continue
		}
		devicesOn[device.Entity.ID]++
		if devicesOn[device.Entity.ID] > 1 {
			return device.Entity.ID, true
		}
	}
	return "", false
}

func (s Site) checkHeartbeats(inventory Inventory) error {
	seen := map[string]bool{}
	for _, rule := range s.Heartbeats {
		if seen[rule.Kind] {
			return fmt.Errorf("%w: heartbeat of kind %q", ErrSiteRuleTwice, rule.Kind)
		}
		if err := s.checkHeartbeat(rule, inventory); err != nil {
			return err
		}
		seen[rule.Kind] = true
	}
	return nil
}

func (s Site) checkHeartbeat(rule HeartbeatRule, inventory Inventory) error {
	if !inventory.Commissioned(rule.Kind) {
		return fmt.Errorf("%w: heartbeat of kind %q", ErrSiteKindNotCommissioned, rule.Kind)
	}
	if entity, shared := s.sharedEntity(rule.Kind, func(string) bool { return false }); shared {
		return fmt.Errorf("%w: heartbeat of kind %q on entity %q; a heartbeat rule needs one device of its kind per entity", ErrSiteRuleShared, rule.Kind, entity)
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
