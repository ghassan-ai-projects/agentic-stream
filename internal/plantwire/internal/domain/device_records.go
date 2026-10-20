package domain

import "time"

const qualityGood = "good"

type DeviceBirth struct {
	Device      string        `json:"device"`
	Kind        string        `json:"kind"`
	Board       string        `json:"board"`
	BootID      string        `json:"boot_id"`
	BirthTimeNS int64         `json:"birth_time_ns"`
	Metrics     []BirthMetric `json:"metrics"`
}

type BirthMetric struct {
	Name    string `json:"name"`
	Alias   uint64 `json:"alias"`
	Value   any    `json:"value"`
	Quality string `json:"quality"`
}

type DeviceData struct {
	Device     string   `json:"device"`
	BootID     string   `json:"boot_id"`
	Historical bool     `json:"historical"`
	Metrics    []Sample `json:"metrics"`
}

type Sample struct {
	Alias        uint64 `json:"alias"`
	Value        any    `json:"value"`
	Quality      string `json:"quality"`
	EventTimeNS  int64  `json:"event_time_ns"`
	DeviceMonoUS *int64 `json:"device_mono_us"`
}

type DeviceDeath struct {
	Device string `json:"device"`
	BootID string `json:"boot_id"`
}

type Sync struct {
	WorldTimeNS int64 `json:"world_time_ns"`
}

func eventTime(ns int64) time.Time {
	return time.Unix(0, ns).UTC()
}
