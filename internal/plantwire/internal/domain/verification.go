package domain

import (
	"maps"
	"slices"
)

const (
	evidenceObserved    = "observed"
	evidenceNotObserved = "not_observed"
	maxEvidence         = 128
)

type Observation struct {
	Expectation
	Observed any
}

type Verification struct {
	CommandID    string
	Device       string
	BootID       string
	Verified     bool
	Observations []Observation
}

type watch struct {
	order    CommandOrder
	expected []Expectation
	observed map[string]any
}

func (c *Conversation) watch(o CommandOrder, r *Reaction) {
	device := c.ingest.devices[o.Device]
	var expected []Expectation
	if device != nil && device.boot == o.ExpectedBootID {
		expected = c.admission.Inventory.Expectations(device.kind, o.Args)
	}
	if len(expected) == 0 {
		r.Verifications = append(r.Verifications, Verification{CommandID: o.CommandID, Device: o.Device, BootID: o.ExpectedBootID})
		return
	}
	c.watches[o.CommandID] = &watch{order: o, expected: expected, observed: map[string]any{}}
}

func (c *Conversation) Unwatch(commandID string) {
	if w := c.watches[commandID]; w != nil {
		c.evidence = append(c.evidence, w.evidence()...)
		delete(c.watches, commandID)
	}
}

func (c *Conversation) verify(a Arrival, r *Reaction) {
	data := a.DeviceData
	if data == nil || data.Historical || len(c.watches) == 0 {
		return
	}
	device := c.ingest.devices[data.Device]
	if device == nil || device.boot != data.BootID {
		return
	}
	for _, sample := range data.Metrics {
		if metric, known := device.aliases[sample.Alias]; known && sample.Quality == qualityGood {
			c.observe(device, metric, sample.Value, r)
		}
	}
}

func (c *Conversation) observe(device *liveDevice, metric string, value any, r *Reaction) {
	for _, id := range slices.Sorted(maps.Keys(c.watches)) {
		w := c.watches[id]
		if w.order.Device == device.name && w.order.ExpectedBootID == device.boot && w.observe(metric, value) {
			r.Verifications = append(r.Verifications, w.verification())
			c.evidence = append(c.evidence, w.evidence()...)
			delete(c.watches, id)
		}
	}
}

func (c *Conversation) barrier(through uint64) map[string]any {
	record := syncRecord(through)
	if count := min(len(c.evidence), maxEvidence); count > 0 {
		record.Fields["command_evidence"] = slices.Clone(c.evidence[:count])
		c.evidence = c.evidence[count:]
	}
	return c.out.stamp(record)
}

func (w *watch) observe(metric string, value any) bool {
	for _, expected := range w.expected {
		if _, seen := w.observed[metric]; !seen && expected.Metric == metric && expected.Matches(value) {
			w.observed[metric] = value
		}
	}
	return len(w.observed) == len(w.expected)
}

func (w *watch) verification() Verification {
	observations := make([]Observation, len(w.expected))
	for i, expected := range w.expected {
		observations[i] = Observation{Expectation: expected, Observed: w.observed[expected.Metric]}
	}
	return Verification{CommandID: w.order.CommandID, Device: w.order.Device, BootID: w.order.ExpectedBootID, Verified: true,
		Observations: observations}
}

func (w *watch) evidence() []map[string]any {
	var entries []map[string]any
	for _, expected := range w.expected {
		if _, numeric := numberOf(expected.Value); !numeric {
			continue
		}
		entry := map[string]any{"command_id": w.order.CommandID, "device": w.order.Device, "boot_id": w.order.ExpectedBootID,
			"metric": expected.Metric, "expected": expected.Value, "tolerance": expected.Tolerance, "outcome": evidenceNotObserved}
		if value, seen := w.observed[expected.Metric]; seen {
			entry["outcome"], entry["observed_value"] = evidenceObserved, value
		}
		entries = append(entries, entry)
	}
	return entries
}

func (v Verification) Feedback() map[string]any {
	readings := make([]any, len(v.Observations))
	for i, observation := range v.Observations {
		readings[i] = map[string]any{"metric": observation.Metric, "expected": observation.Value,
			"tolerance": observation.Tolerance, "observed": observation.Observed}
	}
	return map[string]any{"command_id": v.CommandID, "device": v.Device, "boot_id": v.BootID, "readings": readings}
}
