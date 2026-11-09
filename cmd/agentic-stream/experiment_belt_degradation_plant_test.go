package main

import (
	"fmt"
	"strconv"
)

const (
	beltEmptyAmps    = 2.25
	beltLoadedAmps   = 3.08
	beltWindingC     = 82.4
	beltWindingPerA  = 7.0
	beltStoppedC     = 40.0
	beltVibrationMmS = 1.45
	beltTailRPM      = 76.4
)

var (
	beltDrives          = []string{"D-1", "D-2", "D-3"}
	beltWindingSpread   = []float64{0, -0.3, 0.4}
	beltVibrationSpread = []float64{0, -0.04, 0.03}
)

type beltStory func(minute int) beltDisturbance

type beltDisturbance struct{ wear, heat, surge, drag float64 }

type beltPlant struct {
	setpoint    float64
	beltRunning bool
	running     []bool
	beltDisturbance
}

func healthyBelt() *beltPlant {
	return &beltPlant{setpoint: 100, beltRunning: true, running: []bool{true, true, true}}
}

func (p *beltPlant) feed() float64 {
	if !p.beltRunning {
		return 0
	}
	return conveyorFullFeed * p.setpoint / 100 * (1 + p.surge)
}

func (p *beltPlant) driveAmps(drive int) float64 {
	if !p.beltRunning || !p.running[drive] {
		return 0
	}
	sharing := 0.0
	for _, running := range p.running {
		sharing += onOff(running)
	}
	load := p.feed()/conveyorFullFeed + p.drag
	return (beltEmptyAmps + (beltLoadedAmps-beltEmptyAmps)*load) * float64(len(p.running)) / sharing
}

func (p *beltPlant) driveReadings(drive int) []float64 {
	amps := p.driveAmps(drive)
	if amps == 0 {
		return []float64{0, beltStoppedC, 0, p.runCommand(drive)}
	}
	winding := beltWindingC + beltWindingPerA*(amps-beltLoadedAmps) + beltWindingSpread[drive]
	vibration := beltVibrationMmS*(0.7+0.3*p.feed()/conveyorFullFeed) + beltVibrationSpread[drive]
	if beltDrives[drive] == "D-3" {
		winding, vibration = winding+p.heat, vibration+p.wear
	}
	return []float64{amps, winding, vibration, p.runCommand(drive)}
}

func (p *beltPlant) runCommand(drive int) float64 { return onOff(p.running[drive]) }

func onOff(running bool) float64 {
	if running {
		return 1
	}
	return 0
}

func (p *beltPlant) obey(device, command string, args map[string]any) {
	switch command {
	case "set_feed":
		p.setpoint, _ = strconv.ParseFloat(fmt.Sprint(args["feed_setpoint"]), 64)
	case "stop_belt":
		p.beltRunning = false
	case "stop_drive":
		for i, drive := range beltDrives {
			p.running[i] = p.running[i] && drive != device
		}
	}
}
