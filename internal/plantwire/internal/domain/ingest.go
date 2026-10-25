package domain

import (
	"maps"
	"slices"
	"strings"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/contractsv1"
)

type Tally struct {
	Events     int
	Historical int
	Dropped    int
}

type liveDevice struct {
	name     string
	kind     string
	board    string
	boot     string
	latest   map[string]HeldReading
	entity   *contractsv1.EntityRef
	aliases  map[uint64]string
	good     bool
	goodNS   int64
	beaten   bool
	lastBeat int64
}

type ingest struct {
	mapping Mapping
	kinds   map[string]bool
	boards  map[string]string
	devices map[string]*liveDevice
	nowNS   int64
	tally   Tally
}

func newIngest(mapping Mapping) *ingest {
	return &ingest{mapping: mapping, kinds: map[string]bool{}, boards: map[string]string{}, devices: map[string]*liveDevice{}}
}

func (g *ingest) rebirth(b Birth, quarantined []string) {
	clear(g.devices)
	clear(g.kinds)
	g.boards = boardEnvironments(b)
	for _, kind := range b.Kinds {
		if !slices.Contains(quarantined, kind.Name()) {
			g.kinds[foldedName(kind)] = true
		}
	}
}

func (g *ingest) observe(a Arrival, r *Reaction) {
	switch a.Type {
	case TypeDeviceBirth:
		g.deviceBirth(*a.DeviceBirth, a, r)
	case TypeDeviceData:
		g.data(a, r)
	case TypeDeviceDeath:
		g.deviceDeath(*a.DeviceDeath, r)
	case TypeSync:
		g.advance(a.Sync.WorldTimeNS, a, r)
	}
}

func (g *ingest) deviceBirth(b DeviceBirth, a Arrival, r *Reaction) {
	if reason := g.unborn(b); reason != "" {
		delete(g.devices, b.Device)
		g.drop(r, reason)
		return
	}
	device := bornDevice(b, a.ReceivedAt)
	if entity, found := g.mapping.directory.EntityOf(b.Device); found {
		device.entity = &entity
	}
	g.devices[b.Device] = device
	g.advance(b.BirthTimeNS, a, r)
}

func (g *ingest) unborn(b DeviceBirth) string {
	siteKind, named := g.mapping.directory.KindOf(b.Device)
	switch {
	case !g.kinds[strings.ToLower(b.Kind)]:
		return "a device of a kind the node birth did not commission"
	case named && siteKind != b.Kind:
		return "a device born as another kind than the site file names"
	}
	return ""
}

func bornDevice(b DeviceBirth, receivedAt time.Time) *liveDevice {
	device := &liveDevice{name: b.Device, kind: b.Kind, board: b.Board, boot: b.BootID, aliases: map[uint64]string{}, latest: map[string]HeldReading{}}
	for _, metric := range b.Metrics {
		device.aliases[metric.Alias] = metric.Name
		device.hold(metric.Name, metric.Value, metric.Quality, receivedAt)
		if metric.Quality == qualityGood {
			device.markGood(b.BirthTimeNS)
		}
	}
	return device
}

func (g *ingest) data(a Arrival, r *Reaction) {
	data := *a.DeviceData
	if data.Historical {
		g.tally.Historical++
		r.Dropped = "historical data is not ingested live"
		return
	}
	device := g.devices[data.Device]
	if device == nil || device.boot != data.BootID {
		g.drop(r, "data of a device that is not born on this boot")
		return
	}
	for _, sample := range data.Metrics {
		g.sample(device, sample, a, r)
	}
	g.beat(a, r)
}

func (g *ingest) sample(device *liveDevice, s Sample, a Arrival, r *Reaction) {
	g.nowNS = max(g.nowNS, s.EventTimeNS)
	metric, known := device.aliases[s.Alias]
	if !known {
		return
	}
	device.hold(metric, s.Value, s.Quality, a.ReceivedAt)
	if s.Quality == qualityGood {
		device.markGood(s.EventTimeNS)
	}
	if reading, mapped := g.mapping.reading(device.kind, metric); mapped && device.entity != nil {
		g.emit(r, g.mapping.readingEvent(device, reading, s, a))
	}
}

func (g *ingest) deviceDeath(d DeviceDeath, r *Reaction) {
	device := g.devices[d.Device]
	if device == nil || device.boot != d.BootID {
		g.drop(r, "the death of a device that is not born on this boot")
		return
	}
	delete(g.devices, d.Device)
}

func (g *ingest) advance(ns int64, a Arrival, r *Reaction) {
	g.nowNS = max(g.nowNS, ns)
	g.beat(a, r)
}

func (g *ingest) beat(a Arrival, r *Reaction) {
	for _, name := range slices.Sorted(maps.Keys(g.devices)) {
		device := g.devices[name]
		if rule, due := g.heartbeatDue(device); due {
			device.beaten, device.lastBeat = true, g.nowNS
			g.emit(r, g.mapping.heartbeatEvent(device, rule, g.nowNS, a))
		}
	}
}

func (g *ingest) heartbeatDue(device *liveDevice) (HeartbeatRule, bool) {
	rule, beats := g.mapping.heartbeats[device.kind]
	return rule, beats && device.entity != nil && device.fresh(g.nowNS, rule) && device.due(g.nowNS, rule)
}

func (g *ingest) emit(r *Reaction, event contractsv1.Envelope) {
	g.tally.Events++
	r.Events = append(r.Events, event)
}

func (g *ingest) drop(r *Reaction, reason string) {
	g.tally.Dropped++
	r.Dropped = reason
}

func (d *liveDevice) markGood(ns int64) {
	d.good, d.goodNS = true, max(d.goodNS, ns)
}

func (d *liveDevice) fresh(nowNS int64, rule HeartbeatRule) bool {
	return d.good && nowNS-d.goodNS <= millisecondsInNS(rule.StaleAfterMS)
}

func (d *liveDevice) due(nowNS int64, rule HeartbeatRule) bool {
	return !d.beaten || nowNS-d.lastBeat >= millisecondsInNS(rule.EveryMS)
}

func millisecondsInNS(ms int64) int64 {
	return ms * 1_000_000
}
