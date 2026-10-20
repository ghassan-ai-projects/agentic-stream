package domain

import (
	"fmt"
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
	BootID   string
	Origin   Origin
	Outputs  []string
	Readings []HeldReading
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
