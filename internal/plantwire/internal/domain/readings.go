package domain

import (
	"fmt"
	"slices"
	"time"
)

type HeldReading struct {
	Metric     string
	Value      any
	Good       bool
	BootID     string
	ReceivedAt time.Time
}

type DeviceView struct {
	BootID    string
	Origin    Origin
	Outputs   []string
	Readings  []HeldReading
	NodeNowNS int64
}

func (v DeviceView) ReadsBack(args map[string]any) bool {
	return slices.ContainsFunc(v.Outputs, func(output string) bool {
		_, commanded := args[output]
		return commanded
	})
}

func (c *Conversation) Device(name string) (DeviceView, error) {
	if !c.accepted {
		return DeviceView{}, ErrNoSession
	}
	device := c.ingest.devices[name]
	if device == nil {
		return DeviceView{}, fmt.Errorf("%w: %s", ErrDeviceNotBorn, name)
	}
	outputs := c.admission.Inventory.Outputs(device.kind)
	return DeviceView{BootID: device.boot, Origin: c.ingest.originOf(device), Outputs: outputs, Readings: device.readingsOf(outputs)}, nil
}

func (c *Conversation) NodeNowNS(hostNow time.Time) int64 { return c.ingest.nodeNowNS(hostNow) }

func (c *Conversation) OriginOf(name string) (Origin, error) {
	if !c.accepted {
		return Origin{}, ErrNoSession
	}
	device := c.ingest.devices[name]
	if device == nil {
		return Origin{}, fmt.Errorf("%w: %s", ErrDeviceNotBorn, name)
	}
	return c.ingest.originOf(device), nil
}

func (d *liveDevice) hold(metric string, value any, quality string, receivedAt time.Time) {
	d.latest[metric] = HeldReading{Metric: metric, Value: value, Good: quality == qualityGood, BootID: d.boot, ReceivedAt: receivedAt}
}

func (d *liveDevice) readingsOf(metrics []string) []HeldReading {
	var readings []HeldReading
	for _, metric := range metrics {
		if reading, held := d.latest[metric]; held {
			readings = append(readings, reading)
		}
	}
	return readings
}
