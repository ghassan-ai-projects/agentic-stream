package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/testsupport/plantwirefake"
)

const (
	beltTick        = time.Minute
	windingEvery    = 10 * time.Second
	beltHistory     = 3 * time.Hour
	beltCommandPoll = 100 * time.Millisecond
)

type beltLine struct {
	node   *plantwirefake.Node
	db     *sql.DB
	plant  *beltPlant
	story  beltStory
	steps  map[string]bool
	served map[string]bool
	start  time.Time
	tick   int
	obeyed []string
}

func connectBeltLine(t *testing.T, run degradationServe, story beltStory) *beltLine {
	t.Helper()
	line := &beltLine{node: plantwirefake.Dial(t, "unix", run.socket, "belt-1"), db: openReadOnly(t, run.db), plant: healthyBelt(),
		story: story, steps: plannedSteps, served: map[string]bool{}, start: time.Now().Add(-beltHistory).Truncate(time.Minute)}
	line.node.Send(plantwirefake.Birth(t, conveyorKinds, "mega-01", conveyorDevices...))
	line.node.Expect("STATE")
	born := line.start.Add(-time.Minute).UnixNano()
	line.node.Send(deviceBirth("F-1", "bulk_feeder", born, birthMetric("feed_setpoint", 1, 100, "good"), birthMetric("feed", 2, conveyorFullFeed, "good")))
	line.node.Send(withBoot(deviceBirth("B-1", "belt_conveyor", born, birthMetric("tail_speed", 1, beltTailRPM, "good"), birthMetric("run_command", 2, 1, "good"))))
	for _, drive := range beltDrives {
		line.node.Send(withBoot(deviceBirth(drive, "conveyor_drive", born, birthMetric("current", 1, beltLoadedAmps, "good"),
			birthMetric("winding", 2, beltWindingC, "good"), birthMetric("vibration", 3, beltVibrationMmS, "good"), birthMetric("run_command", 4, 1, "good"))))
	}
	return line
}

func withBoot(birth map[string]any) map[string]any {
	birth["boot_id"] = bootOf(birth["device"].(string))
	return birth
}

func (l *beltLine) runUntil(t *testing.T, ticks int, settled func(now time.Time) bool) bool {
	t.Helper()
	for ; l.tick < ticks; l.tick++ {
		l.advance(t)
		if settled(l.now()) || l.awaitPlannedStep(t, settled) {
			return true
		}
	}
	return false
}

func (l *beltLine) now() time.Time { return l.start.Add(time.Duration(l.tick) * beltTick) }

func (l *beltLine) minutes() int { return int(time.Duration(l.tick) * beltTick / time.Minute) }

func (l *beltLine) advance(t *testing.T) {
	t.Helper()
	l.plant.beltDisturbance, l.plant.minute = l.story(l.minutes()), l.minutes()
	l.sendWindingBetweenMinutes()
	at := l.now().UnixNano()
	l.node.Send(beltData("F-1", at, l.plant.setpoint, l.plant.feed()))
	l.node.Send(beltData("B-1", at, beltTailRPM*onOff(l.plant.beltRunning), onOff(l.plant.beltRunning)))
	for i, drive := range beltDrives {
		l.node.Send(beltData(drive, at, l.plant.driveReadings(i)...))
	}
	l.node.Send(map[string]any{"type": "NSYNC", "lane": "control", "through_seq": 0, "world_time_ns": at})
	l.awaitBarrier(t)
}

func (l *beltLine) sendWindingBetweenMinutes() {
	for sampled := l.now().Add(windingEvery - beltTick); sampled.Before(l.now()); sampled = sampled.Add(windingEvery) {
		for i, drive := range beltDrives {
			winding := benchSample(2, l.plant.driveReadings(i)[1], sampled.UnixNano())
			l.node.Send(map[string]any{"type": "DDATA", "lane": "data", "device": drive, "boot_id": bootOf(drive), "historical": false, "metrics": []any{winding}})
		}
	}
}

func beltData(device string, atNS int64, values ...float64) map[string]any {
	metrics := make([]any, len(values))
	for i, value := range values {
		metrics[i] = benchSample(i+1, value, atNS)
	}
	return map[string]any{"type": "DDATA", "lane": "data", "device": device, "boot_id": bootOf(device), "historical": false, "metrics": metrics}
}

func (l *beltLine) awaitBarrier(t *testing.T) {
	t.Helper()
	for {
		record, read := l.node.Next(loopTimeout)
		if !read {
			t.Fatalf("the host answered no barrier at minute %d", l.minutes())
		}
		switch record["type"] {
		case "HSYNC":
			return
		case "DCMD":
			l.obey(t, record, l.now())
		}
	}
}

func (l *beltLine) awaitPlannedStep(t *testing.T, settled func(now time.Time) bool) bool {
	t.Helper()
	phase := l.phaseOnceProcessed(t)
	if !l.steps[phase] || l.served[phase] {
		return false
	}
	for deadline := time.Now().Add(loopTimeout); time.Now().Before(deadline); {
		if settled(l.now()) {
			return true
		}
		if !l.stepRequested(t, phase) {
			return false
		}
		if record, read := l.node.Next(beltCommandPoll); read && record["type"] == "DCMD" {
			l.obey(t, record, l.now().Add(beltTick))
			l.served[phase] = true
			return false
		}
	}
	t.Fatalf("no command for phase %s at minute %d within %s; obeyed %v", phase, l.minutes(), loopTimeout, l.obeyed)
	return false
}

func (l *beltLine) stepRequested(t *testing.T, phase string) bool {
	t.Helper()
	var steps int
	query := `SELECT COUNT(*) FROM intents i JOIN situation_versions v ON v.situation_id = i.situation_id AND v.version = i.situation_version
		WHERE v.phase = ? AND i.intent_type != 'install_watch_condition'`
	if err := l.db.QueryRowContext(t.Context(), query, phase).Scan(&steps); err != nil {
		t.Fatal(err)
	}
	return steps > 0
}

func (l *beltLine) phaseOnceProcessed(t *testing.T) string {
	t.Helper()
	query := `SELECT (SELECT COALESCE(MAX(last_position), 0) FROM partition_checkpoints WHERE consumer_name = 'engine')
		>= (SELECT COALESCE(MAX(position), 0) FROM event_log), COALESCE((SELECT MAX(phase) FROM situations), '')`
	var phase string
	processed := pollUntil(t, loopTimeout, func() bool {
		var done bool
		return l.db.QueryRowContext(t.Context(), query).Scan(&done, &phase) == nil && done
	})
	if !processed {
		t.Fatalf("the engine did not process minute %d within %s", l.minutes(), loopTimeout)
	}
	return phase
}

func (l *beltLine) obey(t *testing.T, command map[string]any, at time.Time) {
	t.Helper()
	if deadline, err := command["deadline_ns"].(json.Number).Int64(); err != nil || deadline < at.UnixNano() {
		t.Fatalf("the node acts on %v at %s, past its deadline (%v): a route must outlive a world step", command, at, err)
	}
	device, name := command["device"].(string), command["command"].(string)
	args, _ := command["args"].(map[string]any)
	answerFrom(l.node, bootOf(device), command, "DRECEIPT", "status", "accepted")
	answerFrom(l.node, bootOf(device), command, "DRESULT", "reported_outcome", "succeeded")
	l.plant.obey(device, name, args)
	l.obeyed = append(l.obeyed, fmt.Sprintf("%s %s %v", device, name, args))
}
